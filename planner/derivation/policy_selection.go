package derivation
import (
	"github.com/Cloud-Pie/SPDT/types"
	"sort"
	"errors"
	"github.com/Cloud-Pie/SPDT/util"
)

/*Evaluates and select the most suitable policy for the given system configurations and forecast
 in:
	@policies *[]types.Policy
				- List of derived policies
	@sysConfig config.SystemConfiguration
				- Configuration specified by the user in the config file
	@vmProfiles []types.VmProfile
				- List of virtual machines profiles
	@forecast types.Forecast
				- Forecast of the expected load
 out:
	@types.Policy
			- Selected policy
	@error
			- Error in case of any
*/
func SelectPolicy(policies *[]types.Policy, sysConfig util.SystemConfiguration, vmProfiles []types.VmProfile, forecast types.Forecast)(types.Policy, error) {

	mapVMProfiles := VMListToMap(vmProfiles)
	//Calculate total cost of the policy
	for i := range *policies {
		policyMetrics, vmTypes:= ComputePolicyMetrics(&(*policies)[i].ScalingActions,forecast.ForecastedValues, systemConfiguration, mapVMProfiles )
		policyMetrics.StartTimeDerivation = (*policies)[i].Metrics.StartTimeDerivation
		policyMetrics.FinishTimeDerivation = (*policies)[i].Metrics.FinishTimeDerivation
		duration := (*policies)[i].Metrics.FinishTimeDerivation.Sub((*policies)[i].Metrics.StartTimeDerivation).Seconds()
		policyMetrics.DerivationDuration = util.RoundN(duration, 2.0)
		(*policies)[i].Metrics = policyMetrics
		(*policies)[i].Parameters[types.VMTYPES] = MapKeysToString(vmTypes)
	}
	//Sort policies based on price
	sort.Slice(*policies, func(i, j int) bool {
		costi := (*policies)[i].Metrics.Cost
		costj := (*policies)[j].Metrics.Cost

		if costi < costj {
			return true
		}else  if costi == costj {
			return (*policies)[i].Metrics.NumberContainerScalingActions < (*policies)[j].Metrics.NumberContainerScalingActions
		}
		return false
	})

	if len(*policies) >0 {
		remainBudget, time := isEnoughBudget(sysConfig.PricingModel.Budget, (*policies)[0])
		if remainBudget {
			(*policies)[0].Status = types.SELECTED
			return (*policies)[0], nil
		} else {
			return (*policies)[0], errors.New("Budget is not enough for time window, you should increase the budget to ensure resources after " +time.String())
		}
	} else {
		return types.Policy{}, errors.New("No suitable policy found")
	}
}


//Compute the metrics related to the policy and its scaling actions
func ComputePolicyMetrics(scalingActions *[]types.ScalingAction, forecast []types.ForecastedValue,
	sysConfiguration util.SystemConfiguration, mapVMProfiles map[string]types.VmProfile) (types.PolicyMetrics, map[string]bool) {

	var avgOverProvision float64
	var avgUnderProvision float64
	var avgElapsedTime float64
	var avgTransitionTime float64
	var avgShadowTime float64

	totalCost	:= 0.0
	numberVMScalingActions := 0
	numberContainerScalingActions := 0
	vmTypes := make(map[string] bool)
	totalOver := 0.0
	totalUnder := 0.0
	totalElapsedTime := 0.0
	totalTransitionTime := 0.0
	totalShadowTime := 0.0

	index := 0
	numberScalingActions := len(*scalingActions)
	nPredictedValues := len(forecast)
	for i, _ := range *scalingActions {
		scalingAction := (*scalingActions)[i]
		var underProvision float64
		var overProvision float64
		cost := 0.0
		var transitionTime float64
		var elapsedTime float64
		var shadowTime float64
		var cpuUtilization float64
		var memUtilization float64

		//Capacity
		scaleActionOverProvision := 0.0
		scaleActionUnderProvision := 0.0
		numSamplesOver := 0.0
		numSamplesUnder := 0.0
		for  index < nPredictedValues && scalingAction.TimeEnd.After(forecast[index].TimeStamp) {
			deltaLoad := scalingAction.Metrics.RequestsCapacity - forecast[index].Requests
			if deltaLoad > 0 {
				scaleActionOverProvision += deltaLoad*100.0/ forecast[index].Requests
				numSamplesOver++
			} else if deltaLoad < 0 {
				scaleActionUnderProvision += -1*deltaLoad*100.0/ forecast[index].Requests
				numSamplesUnder++
			}
			index++
		}
		if numSamplesUnder > 0 {
			underProvision = util.RoundN(scaleActionUnderProvision/numSamplesUnder, 2.0)
			totalUnder += scaleActionUnderProvision /numSamplesUnder
		}
		if numSamplesOver > 0 {
			overProvision = util.RoundN(scaleActionOverProvision/numSamplesOver, 2.0)
			totalOver += scaleActionOverProvision /numSamplesOver
		}

		//Other metrics
		vmSetDesired := scalingAction.DesiredState.VMs
		vmSetInitial := scalingAction.InitialState.VMs
		if !vmSetDesired.Equal(vmSetInitial) {
			numberVMScalingActions += 1
		}

		desiredServiceReplicas := scalingAction.DesiredState.Services[sysConfiguration.MainServiceName]
		initialServiceReplicas := scalingAction.InitialState.Services[sysConfiguration.MainServiceName]
		if !desiredServiceReplicas.Equal(initialServiceReplicas) {
			numberContainerScalingActions += 1
		}

		totalCPUCoresInVMSet := 0.0
		totalMemGBInVMSet := 0.0
		deltaTime := BilledTime(scalingAction.TimeStart, scalingAction.TimeEnd, sysConfiguration.PricingModel.BillingUnit)
		for k,v := range vmSetDesired {
			vmTypes[k] = true
			totalCPUCoresInVMSet += mapVMProfiles[k].CPUCores * float64(v)
			totalMemGBInVMSet += mapVMProfiles[k].Memory * float64(v)
			totalPrice := mapVMProfiles [k].Pricing.Price * float64(v) * deltaTime
			cost +=  util.RoundN(totalPrice, 2.0)
		}
		totalCost += cost

		if i>1 {
			previousStateEndTime := (*scalingActions)[i-1].TimeEnd
			shadowTime = previousStateEndTime.Sub(scalingAction.TimeStart).Seconds()
			totalShadowTime += shadowTime
			transitionTime = previousStateEndTime.Sub(scalingAction.TimeStartTransition).Seconds()
			totalTransitionTime += transitionTime
		}

		memUtilization = desiredServiceReplicas.Memory * float64(desiredServiceReplicas.Scale) * 100.0 / totalMemGBInVMSet
		cpuUtilization = desiredServiceReplicas.CPU * float64(desiredServiceReplicas.Scale)  * 100.0 / totalCPUCoresInVMSet
		elapsedTime = scalingAction.TimeEnd.Sub(scalingAction.TimeStart).Seconds()
		totalElapsedTime += elapsedTime

		configMetrics := types.ConfigMetrics {
			UnderProvision:    underProvision,
			OverProvision:     overProvision,
			Cost:              cost,
			TransitionTimeSec: transitionTime,
			ElapsedTimeSec:    elapsedTime,
			ShadowTimeSec:     shadowTime,
			RequestsCapacity:  scalingAction.Metrics.RequestsCapacity,
			CPUUtilization:    cpuUtilization,
			MemoryUtilization: memUtilization,
		}
		(*scalingActions)[i].Metrics = configMetrics
	}

	avgOverProvision = totalOver/ float64(numberScalingActions)
	avgUnderProvision = totalUnder / float64(numberScalingActions)
	avgElapsedTime = totalElapsedTime / float64(numberScalingActions)
	avgTransitionTime = totalTransitionTime / float64(numberScalingActions)
	avgShadowTime = totalShadowTime / float64(numberScalingActions)

	return types.PolicyMetrics {
		Cost:	util.RoundN(totalCost, 2.0),
		OverProvision:	util.RoundN(avgOverProvision, 2.0),
		UnderProvision:	util.RoundN(avgUnderProvision, 2.0),
		NumberVMScalingActions:	numberVMScalingActions,
		NumberContainerScalingActions:numberContainerScalingActions,
		NumberScalingActions:numberVMScalingActions,
		AvgElapsedTime:	util.RoundN(avgElapsedTime, 2.0),
		AvgShadowTime:	util.RoundN(avgShadowTime, 2.0),
		AvgTransitionTime:	util.RoundN(avgTransitionTime, 2.0),
	}, vmTypes
}