package mobile

// MobileDirs is a struct-backed DirResolver. Android caller supplies paths from
// Context.getCacheDir() / getFilesDir() / getDir("config", 0).
type MobileDirs struct {
	Cache  string
	Data   string
	Config string
}

func (d MobileDirs) CacheDir() string  { return d.Cache }
func (d MobileDirs) DataDir() string   { return d.Data }
func (d MobileDirs) ConfigDir() string { return d.Config }
