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
func (do *digitalocean) AddImage(url string, hash string) error {
	return nil
}
func (do *digitalocean) RemoveImage() {}

func (do *digitalocean) AuthFields() []string {
	return []string{}
}

func (do *digitalocean) Init(credentials map[string]string) error {
	return nil
}
