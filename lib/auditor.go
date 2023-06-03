package lib

type Auditor interface {
}

type ParameterAuditor interface {
	Run()
}

type AuditorConfig struct {
	AuditPoolSize    int
	PageLoadTimeout  int
	KeepAfterSuccess bool
}
