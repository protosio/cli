package cloud

type scaleway struct {
}

func newScalewayClient() (*scaleway, error) {
	return &scaleway{}, nil

}

func (sw *scaleway) NewInstance()    {}
func (sw *scaleway) DeleteInstance() {}
func (sw *scaleway) StartInstance()  {}
func (sw *scaleway) StopInstance()   {}
func (sw *scaleway) AddImage()       {}