package fuzz

type Fuzzer interface {
	Run()
}

type FuzzerConfig struct {
	URL         string
	Concurrency int
}
