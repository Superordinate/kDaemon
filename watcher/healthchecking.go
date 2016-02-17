package watcher

import (
	"errors"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/klouds/kDaemon/database"
	"github.com/klouds/kDaemon/logging"
	"github.com/klouds/kDaemon/models"
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
	conts, err := CheckContainers()

	CountContainers(conts, nodes)

	job.Complete = true
	currentTime := time.Now().Local()
	logging.Log("HC > HEALTH CHECK COMPLETE AT " + currentTime.String())

}

func CheckNodes() ([]models.Node, error) {
	nodes, err := database.GetNodes()

	if err != nil {
		logging.Log("HC > HEALTHCHECK CANNOT START. THERE ARE NO NODES")
		return nodes, err
	}

	for index, _ := range nodes {
		//Check Node for basic ping
		conn, err := net.DialTimeout("tcp", nodes[index].DIPAddr+":"+nodes[index].DPort, timeout)
		if err != nil {
			nodes[index].State = "DOWN"
			logging.Log("HC > NODE | " + nodes[index].Name + " | IS CURRENTLY NOT ACCESSIBLE")
			database.UpdateNode(&nodes[index])
			continue
		}

		logging.Log("HC > NODE WITH HOSTNAME | " + nodes[index].Name + " | IS HEALTHY")
		nodes[index].State = "DOWN"

		database.UpdateNode(&nodes[index])
		conn.Close()

	}
	return nodes, nil

}

func CheckContainers() ([]models.Container, error) {
	logging.Log("HC > STARTING CONTAINER CHECK")

	containers, err := database.GetContainers()
	if err != nil {
		logging.Log("HC > THERE ARE NO CONTAINERS, SKIPPING HEALTHCHECK")
		return nil, err
	}

	for index, value := range containers {

		node, err := database.GetNode(value.NodeID)

		if err != nil || node.State == "DOWN" {
			logging.Log("HC > NODE ISNT HEALTHY, MIGRATING NODES")

			MigrateContainer(&containers[index])

			continue
		}

		//if node is healthy, check that container is running
		client, err := docker.NewClient(node.DIPAddr + ":" + node.DPort)

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

				MigrateContainer(&containers[index])
				continue
			}
		} else {
			logging.Log("HC> CONTAINER | " + value.Name + " | IS RUNNING AND HEALTHY")
			continue
		}
	}

	return containers, nil
}

func CountContainers(conts []models.Container, nodes []models.Node) error {
	//If container count == 0 reset all counts to zero and return

	if len(conts) <= 0 {

		logging.Log("HC > RESETTING ALL COUNTS TO ZERO")

		for _, value := range nodes {
			value.ContainerCount = 0
			database.UpdateNode(&value)
		}
		return errors.New("No Containers")
	}

	nodeCounts := make(map[string]int)

	logging.Log("HC > COUNTING CONTAINERS")

	//Loop through containers and count the containers belonging to which nodes
	for _, value := range conts {
		nodeCounts[value.NodeID] = nodeCounts[value.NodeID] + 1
	}

	for _, value := range nodes {
		value.ContainerCount = nodeCounts[value.Id]
		database.UpdateNode(&value)
	}

	return nil
}

func MigrateContainer(container *models.Container) {
	//loses data but maintains uptime at the moment
	container.Status = "DOWN"
	database.UpdateContainer(container)
	AddJob("RC", container)
	AddJob("LC", container)
}
