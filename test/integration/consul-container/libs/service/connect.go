// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/hashicorp/consul/api"

	"github.com/hashicorp/consul/test/integration/consul-container/libs/cluster"
	"github.com/hashicorp/consul/test/integration/consul-container/libs/utils"
)

// ConnectContainer
type ConnectContainer struct {
	ctx               context.Context
	container         testcontainers.Container
	ip                string
	appPort           []int
	externalAdminPort int
	internalAdminPort int
	mappedPublicPort  int
	serviceName       string
}

var _ Service = (*ConnectContainer)(nil)

func (g ConnectContainer) Exec(ctx context.Context, cmd []string) (string, error) {
	exitCode, reader, err := g.container.Exec(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("exec with error %s", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("exec with exit code %d", exitCode)
	}
	buf, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("error reading from exec output: %w", err)
	}
	return string(buf), nil
}

func (g ConnectContainer) Export(partition, peer string, client *api.Client) error {
	return fmt.Errorf("ConnectContainer export unimplemented")
}

func (g ConnectContainer) GetAddr() (string, int) {
	return g.ip, g.appPort[0]
}

func (g ConnectContainer) GetAddrs() (string, []int) {
	return g.ip, g.appPort
}

func (g ConnectContainer) GetPort(port int) (int, error) {
	return 0, errors.New("not implemented")
}

func (g ConnectContainer) Restart() error {
	_, err := g.GetStatus()
	if err != nil {
		return fmt.Errorf("error fetching sidecar container state %s", err)
	}

	fmt.Printf("Stopping container: %s\n", g.GetName())
	err = g.container.Stop(g.ctx, nil)

	if err != nil {
		return fmt.Errorf("error stopping sidecar container %s", err)
	}

	fmt.Printf("Starting container: %s\n", g.GetName())
	err = g.container.Start(g.ctx)
	if err != nil {
		return fmt.Errorf("error starting sidecar container %s", err)
	}
	return nil
}

func (g ConnectContainer) GetLogs() (string, error) {
	rc, err := g.container.Logs(context.Background())
	if err != nil {
		return "", fmt.Errorf("could not get logs for connect service %s: %w", g.GetServiceName(), err)
	}
	defer rc.Close()

	out, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("could not read from logs for connect service %s: %w", g.GetServiceName(), err)
	}
	return string(out), nil
}

func (g ConnectContainer) GetName() string {
	name, err := g.container.Name(g.ctx)
	if err != nil {
		return ""
	}
	return name
}

func (g ConnectContainer) GetServiceName() string {
	return g.serviceName
}

func (g ConnectContainer) Start() error {
	if g.container == nil {
		return fmt.Errorf("container has not been initialized")
	}
	return g.container.Start(g.ctx)
}

func (g ConnectContainer) Stop() error {
	if g.container == nil {
		return fmt.Errorf("container has not been initialized")
	}
	return g.container.Stop(context.Background(), nil)
}

func (g ConnectContainer) Terminate() error {
	return cluster.TerminateContainer(g.ctx, g.container, true)
}

func (g ConnectContainer) GetInternalAdminAddr() (string, int) {
	return "localhost", g.internalAdminPort
}

// GetAdminAddr returns the external admin port
func (g ConnectContainer) GetAdminAddr() (string, int) {
	return "localhost", g.externalAdminPort
}

func (g ConnectContainer) GetStatus() (string, error) {
	state, err := g.container.State(g.ctx)
	return state.Status, err
}

type SidecarConfig struct {
	Name      string
	ServiceID string
	Namespace string
}

// NewConnectService returns a container that runs envoy sidecar, launched by
// "consul connect envoy", for service name (serviceName) on the specified
// node. The container exposes port serviceBindPort and envoy admin port
// (19000) by mapping them onto host ports. The container's name has a prefix
// combining datacenter and name.
func NewConnectService(ctx context.Context, sidecarCfg SidecarConfig, serviceBindPorts []int, node cluster.Agent) (*ConnectContainer, error) {
	nodeConfig := node.GetConfig()
	if nodeConfig.ScratchDir == "" {
		return nil, fmt.Errorf("node ScratchDir is required")
	}

	namePrefix := fmt.Sprintf("%s-service-connect-%s", node.GetDatacenter(), sidecarCfg.Name)
	containerName := utils.RandName(namePrefix)

	envoyVersion := getEnvoyVersion()
	agentConfig := node.GetConfig()
	buildargs := map[string]*string{
		"ENVOY_VERSION": utils.StringToPointer(envoyVersion),
		"CONSUL_IMAGE":  utils.StringToPointer(agentConfig.DockerImage()),
	}

	dockerfileCtx, err := getDevContainerDockerfile()
	if err != nil {
		return nil, err
	}
	dockerfileCtx.BuildArgs = buildargs

	internalAdminPort, err := node.ClaimAdminPort()
	if err != nil {
		return nil, err
	}

	req := testcontainers.ContainerRequest{
		FromDockerfile: dockerfileCtx,
		WaitingFor:     wait.ForLog("").WithStartupTimeout(10 * time.Second),
		AutoRemove:     false,
		Name:           containerName,
		Cmd: []string{
			"consul", "connect", "envoy",
			"-sidecar-for", sidecarCfg.ServiceID,
			"-admin-bind", fmt.Sprintf("0.0.0.0:%d", internalAdminPort),
			"-namespace", sidecarCfg.Namespace,
			"--",
			"--log-level", envoyLogLevel,
		},
		Env: make(map[string]string),
	}

	nodeInfo := node.GetInfo()
	if nodeInfo.UseTLSForAPI || nodeInfo.UseTLSForGRPC {
		req.Mounts = append(req.Mounts, testcontainers.ContainerMount{
			Source: testcontainers.DockerBindMountSource{
				// See cluster.NewConsulContainer for this info
				HostPath: filepath.Join(nodeConfig.ScratchDir, "ca.pem"),
			},
			Target:   "/ca.pem",
			ReadOnly: true,
		})
	}

	if nodeInfo.UseTLSForAPI {
		req.Env["CONSUL_HTTP_ADDR"] = fmt.Sprintf("https://127.0.0.1:%d", 8501)
		req.Env["CONSUL_HTTP_SSL"] = "1"
		req.Env["CONSUL_CACERT"] = "/ca.pem"
	} else {
		req.Env["CONSUL_HTTP_ADDR"] = fmt.Sprintf("http://127.0.0.1:%d", 8500)
	}

	if nodeInfo.UseTLSForGRPC {
		req.Env["CONSUL_GRPC_ADDR"] = fmt.Sprintf("https://127.0.0.1:%d", 8503)
		req.Env["CONSUL_GRPC_CACERT"] = "/ca.pem"
	} else {
		req.Env["CONSUL_GRPC_ADDR"] = fmt.Sprintf("http://127.0.0.1:%d", 8502)
	}

	var (
		appPortStrs  []string
		adminPortStr = strconv.Itoa(internalAdminPort)
	)

	for _, port := range serviceBindPorts {
		appPortStrs = append(appPortStrs, strconv.Itoa(port))
	}

	// expose the app ports and the envoy adminPortStr on the agent container
	exposedPorts := make([]string, len(appPortStrs))
	copy(exposedPorts, appPortStrs)
	exposedPorts = append(exposedPorts, adminPortStr)
	info, err := cluster.LaunchContainerOnNode(ctx, node, req, exposedPorts)
	if err != nil {
		return nil, err
	}

	out := &ConnectContainer{
		ctx:               ctx,
		container:         info.Container,
		ip:                info.IP,
		externalAdminPort: info.MappedPorts[adminPortStr].Int(),
		internalAdminPort: internalAdminPort,
		serviceName:       sidecarCfg.Name,
	}

	for _, port := range appPortStrs {
		out.appPort = append(out.appPort, info.MappedPorts[port].Int())
	}

	fmt.Printf("NewConnectService: name %s, mapped App Port %d, service bind port %v\n",
		sidecarCfg.ServiceID, out.appPort, serviceBindPorts)
	fmt.Printf("NewConnectService sidecar: name %s, mapped admin port %d, admin port %d\n",
		sidecarCfg.Name, out.externalAdminPort, internalAdminPort)

	return out, nil
}
