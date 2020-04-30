package network

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strings"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const (
	wgRunPath = "/var/run/wireguard"
	wgGoPath  = "/usr/local/bin/wireguard-go"
)

//
// link implements the Link interface
//

type linkTUN struct {
	name              string
	realInterface     string
	interfaceNameFile string
	interfaceSockFile string
}

func (l *linkTUN) Interface() net.Interface {
	return net.Interface{}
}
func (l *linkTUN) Name() string {
	return l.name
}
func (l *linkTUN) Index() int {
	return 0
}

func (l *linkTUN) IsUp() bool {
	return false
}
func (l *linkTUN) SetUp(bool) error {
	// the userspace implementation on MacOS is always up once it's created
	return nil
}
func (l *linkTUN) Addrs() ([]Address, error) {
	return []Address{}, nil
}
func (l *linkTUN) DelAddr(a Address) error {
	return nil
}
func (l *linkTUN) AddAddr(a Address) error {
	return nil
}

func (l *linkTUN) ConfigureWG(wgtypes.Config) error {
	return nil
}
func (l *linkTUN) WGConfig() (*wgtypes.Device, error) {
	return &wgtypes.Device{}, nil
}

func (l *linkTUN) AddRoute(Route) error {
	return nil
}
func (l *linkTUN) DelRoute(Route) error {
	return nil
}

//
// linkMngr implements the Manager interface
//

type linkMngr struct {
	wg *wgctrl.Client
}

func (m *linkMngr) Links() ([]Link, error) {

	// retrieve all files in the wireguard run path
	f, err := os.Open(wgRunPath)
	if err != nil {
		return []Link{}, fmt.Errorf("failed to retrieve wireguard links: %w", err)
	}
	files, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return []Link{}, fmt.Errorf("failed to retrieve wireguard links: %w", err)
	}

	// make list of interface files
	interfaceNameFiles := []string{}
	for _, file := range files {
		if strings.Contains(file.Name(), ".name") {
			interfaceNameFiles = append(interfaceNameFiles, strings.TrimSuffix(file.Name(), ".name"))
		}
	}

	// make list of links
	links := []Link{}
	for _, name := range interfaceNameFiles {
		lnk, err := m.GetLink(name)
		if err != nil {
			return []Link{}, fmt.Errorf("failed to retrieve wireguard links: %w", err)
		}
		links = append(links, lnk)
	}

	return links, nil
}

func (m *linkMngr) CreateLink(name string) (Link, error) {
	_, err := m.GetLink(name)
	if err == nil {
		return &linkTUN{}, fmt.Errorf("failed to create link using wireguard-go: link '%s' already exists", name)
	}

	interfaceFile := fmt.Sprintf("%s/%s.name", wgRunPath, name)
	additionalEnv := fmt.Sprintf("WG_TUN_NAME_FILE=%s", interfaceFile)
	newEnv := append(os.Environ(), additionalEnv)

	// execute wireguard-go
	cmd := exec.Command(wgGoPath, "utun")
	cmd.Env = newEnv
	err = cmd.Run()
	if err != nil {
		return &linkTUN{}, fmt.Errorf("failed to create link using wireguard-go: %w", err)
	}

	// read interface file and figure out the real interface
	sockData, err := ioutil.ReadFile(interfaceFile)
	if err != nil {
		return &linkTUN{}, fmt.Errorf("failed to create link using wireguard-go: %w", err)
	}
	realInterfaceSock := strings.TrimSuffix(string(sockData), "\n")
	if realInterfaceSock == "" {
		return &linkTUN{}, fmt.Errorf("failed to create link using wireguard-go: '%s' contains invalid data", interfaceFile)
	}

	return m.GetLink(name)
}

func (m *linkMngr) DelLink(name string) error {
	lnk, err := m.GetLink(name)
	if err != nil {
		return err
	}

	link := lnk.(*linkTUN)

	// remove the sock file which will lead to the shutdown of wireguard-go
	err = os.Remove(link.interfaceSockFile)
	if err != nil {
		return fmt.Errorf("could not delete link '%s': %w", name, err)
	}

	// remove the .name file
	err = os.Remove(link.interfaceNameFile)
	if err != nil {
		return fmt.Errorf("could not delete link '%s': %w", name, err)
	}
	return nil
}

func (m *linkMngr) GetLink(name string) (Link, error) {
	interfaceFile := fmt.Sprintf("%s/%s.name", wgRunPath, name)

	// read interface file and figure out the real interface
	sockData, err := ioutil.ReadFile(interfaceFile)
	if err != nil {
		return &linkTUN{}, fmt.Errorf("failed to find link '%s': %w", name, err)
	}
	realInterface := strings.TrimSuffix(string(sockData), "\n")
	if realInterface == "" {
		return &linkTUN{}, fmt.Errorf("failed to find link '%s': '%s' contains invalid data", name, interfaceFile)
	}

	return &linkTUN{
		name:              name,
		realInterface:     realInterface,
		interfaceNameFile: interfaceFile,
		interfaceSockFile: fmt.Sprintf("%s/%s.sock", wgRunPath, realInterface),
	}, nil
}

func (m *linkMngr) Close() error {
	return m.wg.Close()
}

// NewManager returns a link manager based on the wireguard-go userspace implementation
func NewManager() (Manager, error) {
	_, err := os.Stat(wgRunPath)
	if os.IsNotExist(err) {
		err := os.Mkdir(wgRunPath, 0755)
		if err != nil {
			return nil, fmt.Errorf("link mngr: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("link mngr: %w", err)
	}

	wg, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("link mngr: %w", err)
	}
	return &linkMngr{wg: wg}, nil
}
