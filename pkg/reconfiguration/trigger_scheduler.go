package reconfiguration

import (
	"fmt"
	"github.com/Cloud-Pie/SPDT/internal/types"
	"github.com/Cloud-Pie/SPDT/internal/rest_clients/scheduler"
)

func TriggerScheduler(policy types.Policy){
	for _, conf := range policy.Configurations {
		err := scheduler.CreateState(conf.State)
		if err != nil {
			fmt.Printf("The scheduler request failed with error %s\n", err)
			panic(err)
		}
	}
}
