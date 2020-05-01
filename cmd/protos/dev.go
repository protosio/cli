package main

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/protosio/cli/internal/cloud"
	"github.com/protosio/cli/internal/network"
	"github.com/protosio/cli/internal/ssh"
	"github.com/protosio/cli/internal/user"
	pclient "github.com/protosio/protos/pkg/client"
	"github.com/protosio/protos/pkg/types"
	"github.com/urfave/cli/v2"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var cmdDev *cli.Command = &cli.Command{
	Name:  "dev",
	Usage: "Subcommands used for development purposes",
	Subcommands: []*cli.Command{
		{
			Name:      "init",
			ArgsUsage: "<instance name> <key> <ip>",
			Usage:     "Creates a tunnel to a developmnet instance",
			Action: func(c *cli.Context) error {
				name := c.Args().Get(0)
				if name == "" {
					cli.ShowSubcommandHelp(c)
					os.Exit(1)
				}

				key := c.Args().Get(1)
				if key == "" {
					cli.ShowSubcommandHelp(c)
					os.Exit(1)
				}

				ip := c.Args().Get(2)
				if ip == "" {
					cli.ShowSubcommandHelp(c)
					os.Exit(1)
				}
				return devInit(name, key, ip)
			},
		},
		{
			Name:      "vpn",
			ArgsUsage: "<instance>",
			Usage:     "Tunnel to dev instance",
			Action: func(c *cli.Context) error {
				instanceName := c.Args().Get(0)
				if instanceName == "" {
					cli.ShowSubcommandHelp(c)
					os.Exit(1)
				}

				return devTunnel(instanceName)
			},
		},
	},
}

func devInit(instanceName string, keyFile string, ipString string) error {
	usr, err := user.Get(envi)
	if err != nil {
		return err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	instanceInfo := cloud.InstanceInfo{
		VMID:          instanceName,
		PublicIP:      ipString,
		Name:          instanceName,
		CloudType:     cloud.Hyperkit,
		CloudName:     hostname,
		Location:      hostname,
		ProtosVersion: "dev",
	}

	ip := net.ParseIP(ipString)
	if ip == nil {
		return fmt.Errorf("String '%s' is not a valid IP address", ipString)
	}

	auth, err := ssh.NewAuthFromKeyFile(keyFile)
	if err != nil {
		return err
	}

	// allocate network for dev instance
	instances, err := envi.DB.GetAllInstances()
	if err != nil {
		return fmt.Errorf("Failed to allocate network for instance '%s': %w", "dev", err)
	}
	developmentNetwork, err := user.AllocateNetwork(instances)
	if err != nil {
		return fmt.Errorf("Failed to allocate network for instance '%s': %w", "dev", err)
	}

	log.Infof("Creating SSH tunnel to dev instance IP '%s'", ipString)
	tunnel := ssh.NewTunnel(ip.String()+":22", "root", auth, "localhost:8080", log)
	localPort, err := tunnel.Start()
	if err != nil {
		return errors.Wrap(err, "Error while creating the SSH tunnel")
	}

	// wait for the API to be up
	err = cloud.WaitForHTTP(fmt.Sprintf("http://127.0.0.1:%d/ui/", localPort), 20)
	if err != nil {
		return errors.Wrap(err, "Failed to deploy instance")
	}
	log.Infof("Tunnel to '%s' ready", ipString)

	user, err := user.Get(envi)
	if err != nil {
		return err
	}

	// do the initialization
	log.Infof("Initializing instance at '%s'", ipString)
	protos := pclient.NewInitClient(fmt.Sprintf("127.0.0.1:%d", localPort), user.Username, user.Password)
	key, err := ssh.NewKeyFromSeed(usr.Device.KeySeed)
	if err != nil {
		panic(err)
	}

	usrDev := types.UserDevice{
		Name:      usr.Device.Name,
		PublicKey: key.PublicWG().String(),
		Network:   usr.Device.Network,
	}

	// Doing the instance initialization which returns the internal wireguard IP and the public key created using the wireguard library.
	instanceIP, instancePublicKey, err := protos.InitInstance(user.Name, developmentNetwork.String(), user.Domain, []types.UserDevice{usrDev})
	if err != nil {
		return errors.Wrap(err, "Error while doing the instance initialization")
	}
	instanceInfo.InternalIP = instanceIP.String()
	instanceInfo.PublicKey = instancePublicKey
	instanceInfo.Network = developmentNetwork.String()

	err = envi.DB.SaveInstance(instanceInfo)
	if err != nil {
		return errors.Wrapf(err, "Failed to save dev instance '%s'", instanceName)
	}

	// close the SSH tunnel
	err = tunnel.Close()
	if err != nil {
		return errors.Wrap(err, "Error while terminating the SSH tunnel")
	}
	log.Infof("Instance at '%s' is ready", ipString)

	return nil
}

func devTunnel(instanceName string) error {

	usr, err := user.Get(envi)
	if err != nil {
		return err
	}

	manager, err := network.NewManager()
	if err != nil {
		return err
	}

	// create protos vpn interface and configure the address
	lnk, err := manager.CreateLink("protos0")
	if err != nil {
		return err
	}
	ip, netp, err := net.ParseCIDR(usr.Device.Network)
	if err != nil {
		return err
	}
	netp.IP = ip
	err = lnk.AddAddr(network.Address{IPNet: *netp})
	if err != nil {
		return err
	}

	// create wireguard peer configurations and route list
	instances, err := envi.DB.GetAllInstances()
	if err != nil {
		return err
	}
	keepAliveInterval := 25 * time.Second
	peers := []wgtypes.PeerConfig{}
	routes := []network.Route{}
	for _, instance := range instances {
		var pubkey wgtypes.Key
		copy(pubkey[:], instance.PublicKey)

		_, instanceNetwork, err := net.ParseCIDR(instance.Network)
		if err != nil {
			return fmt.Errorf("Failed to parse network for instance '%s': %w", instance.Name, err)
		}
		instanceIP := net.ParseIP(instance.PublicIP)
		if instanceIP == nil {
			return fmt.Errorf("Failed to parse IP for instance '%s'", instance.Name)
		}
		routes = append(routes, network.Route{Dest: *instanceNetwork})

		peerConf := wgtypes.PeerConfig{
			PublicKey:                   pubkey,
			PersistentKeepaliveInterval: &keepAliveInterval,
			Endpoint:                    &net.UDPAddr{IP: instanceIP, Port: 10999},
			AllowedIPs:                  []net.IPNet{*instanceNetwork},
		}
		peers = append(peers, peerConf)
	}

	// configure wireguard
	var pkey wgtypes.Key
	copy(pkey[:], usr.Device.KeySeed)
	wgcfg := wgtypes.Config{
		PrivateKey: &pkey,
		Peers:      peers,
	}
	err = lnk.ConfigureWG(wgcfg)
	if err != nil {
		return err
	}

	// add the routes towards instances
	for _, route := range routes {
		err = lnk.AddRoute(route)
		if err != nil {
			return err
		}
	}

	return nil
}
