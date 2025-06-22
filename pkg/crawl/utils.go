package crawl

import (
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/go-rod/rod"
)

// DismissPagePopups tries to find and click on elements that are likely to be dismissible pop-ups.
func DismissPagePopups(page *rod.Page) error {
	selectors := []string{
		"[aria-label='close']", "[aria-label='Close']",
		"[aria-label='dismiss']", "[aria-label='Dismiss']",
		"button[class*='close']", "button[class*='dismiss']",
		"button[title='Close']", "button[title='Dismiss']",
		".modal-close", ".popup-close", ".close-icon", ".icon-close",
	}

	dismissTexts := []string{
		"close", "schließen", "cerrar", "fermer", "chiudi",
		"dismiss", "cancel", "annuler", "abbrechen", "cancelar",
		"accept", "akzeptieren", "aceptar", "accetta",
		"ok", "okay", "confirm", "bestätigen", "confirmar",
		"no thanks", "no, thanks", "nicht danke", "non merci", "no grazie",
	}

	for _, selector := range selectors {
		elements, err := page.Elements(selector)
		if err != nil {
			continue
		}
		for _, element := range elements {
			text, err := element.Text()
			if err != nil {
				continue
			}
			text = strings.ToLower(text)
			for _, dismissText := range dismissTexts {
				if strings.Contains(text, dismissText) {
					// page.Activate()
					element.Click(proto.InputMouseButtonLeft, 1)
					time.Sleep(1 * time.Second)
					break
				}
			}
		}
	}

	additionalSelectors := []string{
		"div[role='dialog'] button:first-child",     // Often the close button in modals
		"div[class*='overlay'] div[class*='close']", // Overlay close icons
	}

	for _, sel := range additionalSelectors {
		element, err := page.Element(sel)
		if err == nil {
			element.Click(proto.InputMouseButtonLeft, 1)
			time.Sleep(1 * time.Second)
		}

	}
	return nil
}
