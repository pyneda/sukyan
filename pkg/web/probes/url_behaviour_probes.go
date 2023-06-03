package probes

import (
	"github.com/pyneda/sukyan/pkg/fuzz"

	"github.com/rs/zerolog/log"
)

type URLBehaviourProbes struct {
	URL string
}

func (p *URLBehaviourProbes) Run() {
	ipg := fuzz.InjectionPointGatherer{
		ParamsExtensive: false,
	}
	injectionPoints := ipg.GetFromURL(p.URL)
	for _, injectionPoint := range injectionPoints {
		log.Debug().Interface("injection_point", injectionPoint).Msg("injection point")
	}
}

func (p *URLBehaviourProbes) TestURLInjectionPoint(injectionPoint fuzz.URLInjectionPoint) {
	// should use pageloader once properly implemented

	// check for reflection / if appears in DOM
}
