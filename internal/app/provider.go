package app

const (
	appDS = "app"
)

type App struct {
}

// Provider manages apps
type Provider interface {
	GetApps() []App
	SearchAppStore(term string) ([]App, error)
}

// NewProvider creates and returns a new app provider
func NewProvider() Provider {
	return &provider{}
}

type provider struct {
}

func (p *provider) GetApps() []App {
	return []App{}
}

func (p *provider) SearchAppStore(term string) ([]App, error) {
	return []App{}, nil
}
