package cloud

type digitalocean struct {
}

func newDigitalOceanClient() (*digitalocean, error) {
	return &digitalocean{}, nil

}

func (do *digitalocean) NewInstance()    {}
func (do *digitalocean) DeleteInstance() {}
func (do *digitalocean) StartInstance()  {}
func (do *digitalocean) StopInstance()   {}
func (do *digitalocean) AddImage()       {}
