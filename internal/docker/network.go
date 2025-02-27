package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types"
	"github.com/docker/go-connections/nat"
)

type ExposedPorts struct {
	Port     string
	Protocol string
}

type portConfig struct {
	PortBindings nat.PortMap
	PortSet      nat.PortSet
}

func (d *DockerClient) getNetworkConfig(ports []ExposedPorts) portConfig {

	portBindings := make(nat.PortMap)
	portSet := make(nat.PortSet)

	for _, port := range ports {

		portName, err := nat.NewPort(port.Protocol, port.Port)
		if err != nil {
			panic(err)
		}

		portBindings[portName] = []nat.PortBinding{
			{
				HostPort: port.Port,
			},
		}

		portSet[portName] = struct{}{}

	}

	return portConfig{
		PortBindings: portBindings,
		PortSet:      portSet,
	}
}

func (d *DockerClient) EnsureNetwork(name string) (created bool, network types.NetworkResource, err error) {

	hasNetwork, network, err := d.findNetworkByName(name)

	if err != nil {
		return false, types.NetworkResource{}, err
	}

	if hasNetwork {
		return false, network, nil
	}

	networkCreateResults, err := d.client.NetworkCreate(context.Background(), name, types.NetworkCreate{
		Driver: "bridge",
	})

	if err != nil {
		return false, types.NetworkResource{}, err
	}

	hasNetwork, network, err = d.findNetworkById(networkCreateResults.ID)

	if err != nil {
		return false, types.NetworkResource{}, err
	}

	if hasNetwork {
		return true, network, nil
	}

	return false, types.NetworkResource{}, fmt.Errorf("could not create network")
}

func (d *DockerClient) RemoveNetwork(name string) (removed bool, err error) {

	hasNetwork, network, err := d.findNetworkByName(name)

	if err != nil {
		return false, err
	}

	if !hasNetwork {
		return false, nil
	}

	return true, d.client.NetworkRemove(context.Background(), network.ID)
}

func (d *DockerClient) findNetworkByName(name string) (found bool, network types.NetworkResource, err error) {

	networks, err := d.client.NetworkList(context.Background(), types.NetworkListOptions{})

	if err != nil {
		return false, types.NetworkResource{}, err
	}

	for _, n := range networks {
		if n.Name == name {
			return true, n, nil
		}
	}

	return false, types.NetworkResource{}, nil
}

func (d *DockerClient) findNetworkById(ID string) (found bool, network types.NetworkResource, err error) {

	networks, err := d.client.NetworkList(context.Background(), types.NetworkListOptions{})

	if err != nil {
		return false, types.NetworkResource{}, err
	}

	for _, n := range networks {
		if n.ID == ID {
			return true, n, nil
		}
	}

	return false, types.NetworkResource{}, nil
}
