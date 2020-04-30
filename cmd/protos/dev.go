package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/protosio/cli/internal/cloud"
	"github.com/protosio/cli/internal/ssh"
	"github.com/protosio/cli/internal/user"
	pclient "github.com/protosio/protos/pkg/client"
	"github.com/protosio/protos/pkg/types"
	"github.com/urfave/cli/v2"

	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/ipc"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl"
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
			Name:      "tunnel",
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
	interfaceName := "utun6"

	tun, err := tun.CreateTUN(interfaceName, device.DefaultMTU)
	if err != nil {
		return err
	}

	logger := device.NewLogger(
		device.LogLevelDebug,
		fmt.Sprintf("(%s) ", interfaceName),
	)

	fileUAPI, err := func() (*os.File, error) {
		uapiFdStr := os.Getenv("WG_UAPI_FD")
		if uapiFdStr == "" {
			return ipc.UAPIOpen(interfaceName)
		}

		// use supplied fd

		fd, err := strconv.ParseUint(uapiFdStr, 10, 32)
		if err != nil {
			return nil, err
		}

		return os.NewFile(uintptr(fd), ""), nil
	}()
	if err != nil {
		return err
	}

	device := device.NewDevice(tun, logger)

	logger.Info.Println("Device started")

	errs := make(chan error)
	term := make(chan os.Signal, 1)

	uapi, err := ipc.UAPIListen(interfaceName, fileUAPI)
	if err != nil {
		logger.Error.Println("Failed to listen on uapi socket:", err)
		os.Exit(1)
	}

	go func() {
		for {
			conn, err := uapi.Accept()
			if err != nil {
				errs <- err
				return
			}
			go device.IpcHandle(conn)
		}
	}()

	logger.Info.Println("UAPI listener started")

	signal.Notify(term, syscall.SIGTERM)
	signal.Notify(term, os.Interrupt)

	// create wg controller
	wg, err := wgctrl.New()
	if err != nil {
		return err
	}

	user, err := user.Get(envi)
	if err != nil {
		return err
	}

	instances, err := envi.DB.GetAllInstances()
	if err != nil {
		return err
	}
	keepAliveInterval := 20 * time.Second
	peers := []wgtypes.PeerConfig{}
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

		peerConf := wgtypes.PeerConfig{
			PublicKey:                   pubkey,
			PersistentKeepaliveInterval: &keepAliveInterval,
			Endpoint:                    &net.UDPAddr{IP: instanceIP, Port: 10999},
			AllowedIPs:                  []net.IPNet{*instanceNetwork},
		}
		peers = append(peers, peerConf)
	}

	var pkey wgtypes.Key
	copy(pkey[:], user.Device.KeySeed)
	wgcfg := wgtypes.Config{
		PrivateKey: &pkey,
		Peers:      peers,
	}
	err = wg.ConfigureDevice(interfaceName, wgcfg)
	if err != nil {
		return err
	}

	// wait for tunnel to terminate
	select {
	case <-term:
	case <-errs:
	case <-device.Wait():
	}

	// clean up

	uapi.Close()
	device.Close()

	logger.Info.Println("Shutting down")

	return nil
}
