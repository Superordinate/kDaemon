package watcher

import (
	"github.com/superordinate/kDaemon/logging"
	"github.com/superordinate/kDaemon/database"
	"github.com/superordinate/kDaemon/models"
	docker "github.com/fsouza/go-dockerclient"
	"net"
	"time"
)

const timeout = time.Duration(5) * time.Second

func PerformHealthCheck(job *Job) {
	job.InUse = true

	//Check if nodes are up
	nodes, err := CheckNodes()

	if err != nil || len(nodes) == 0 {
		logging.Log("HC > CANCEL HEALTHCHECK, NO HEALTHY NODES")
		job.Complete = true
		return
	}
	//Check if containers are running
	CheckContainers()

	job.Complete = true
	
}

func CheckNodes() ([]models.Node, error) {
	nodes, err := database.GetNodes()

	if err != nil {
		logging.Log("HC > HEALTHCHECK CANNOT START. THERE ARE NO NODES")
		return nodes, err
	}

	for _, value := range nodes {
		//Check Node for basic ping

		conn, err := net.DialTimeout("tcp", value.DIPAddr + ":" + value.DPort, timeout)
		if err != nil {
			value.IsEnabled = false
			value.IsHealthy = false
			logging.Log("HC > NODE | " + value.Hostname + " | IS CURRENTLY NOT ACCESSIBLE")
			database.UpdateNode(&value)
			continue;
		} 
		logging.Log("HC > NODE WITH HOSTNAME | " + value.Hostname + " | IS HEALTHY")
		value.IsHealthy = true
		value.IsEnabled = true

		database.UpdateNode(&value)
		conn.Close()

	}

	return nodes, nil

}

func CheckContainers() error{
	logging.Log("HC > STARTING CONTAINER CHECK")
	
	containers, err := database.GetContainers()
	if err != nil {
		logging.Log("HC > THERE ARE NO CONTAINERS, SKIPPING HEALTHCHECK")
		return err
	}

	for index, value := range containers {

		node, err := database.GetNode(value.NodeID)

		if err != nil || node.IsHealthy == false {
			logging.Log ("HC > NODE ISNT HEALTHY, MIGRATING NODES")
			
				node.ContainerCount = node.ContainerCount - 1

			if node.ContainerCount <= 0 {
				node.ContainerCount = 0
			}

			logging.Log("HC > NODE INFORMATION")
			logging.Log(node)
			database.UpdateNode(node)
			
			MigrateContainer(&containers[index])

			continue;
		}

		//if node is healthy, check that container is running
		client,err := docker.NewClient(node.DIPAddr + ":" + node.DPort)

		if err != nil {
			logging.Log(err)
			continue
		}

		dock_cont, err := client.InspectContainer(value.ContainerID)

		if err != nil {
			logging.Log("HC > CONTAINER | " + value.Name + " | DOESNT EXIST")
			value.Status = "DOWN"

			AddJob("LC", containers[index])
			continue
		}

		//Check if container is already running
		if dock_cont.State.Running == false { 
			//if its not running, attempt to start container
			//start container
			err = client.StartContainer(value.ContainerID, nil)

			//if container doesn't start, attempt migration
		    if err != nil {
		    	logging.Log("HC > CONTAINER WONT START, | " + value.Name + " | MIGRATING")

		    	node.ContainerCount = node.ContainerCount - 1

				if node.ContainerCount <= 0 {
					node.ContainerCount = 0
				}

				logging.Log("HC > NODE INFORMATION")
				logging.Log(node)
				database.UpdateNode(node)

		        MigrateContainer(&containers[index])
		        continue
		    }
		} else {
			logging.Log("HC> CONTAINER | " + value.Name + " | IS RUNNING AND HEALTHY")
			continue
		}		
	}

	return nil
}

func MigrateContainer(container *models.Container) {
	//loses data but maintains uptime at the moment
	container.NodeID = 0
	container.Status = "DOWN"
	container.IsEnabled = false

	database.UpdateContainer(container)

	AddJob("LC", container)
}