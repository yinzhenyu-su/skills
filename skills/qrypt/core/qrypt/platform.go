package qrypt

type DirResolver interface {
	CacheDir() string
	DataDir() string
	ConfigDir() string
}

type CredentialStore interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}
