package browser

import (
	"fmt"

	"github.com/pyneda/sukyan/pkg/payloads"
)

func GetTaintTrackingScript(marker string) string {
	return fmt.Sprintf(`
(function() {
    if (window.__sukyanTaintReady) return; // Prevent double injection
    const MARKER = '%s';
    const TAINT_PREFIX = '%s';

    // Wrap innerHTML setter
    const origInnerHTMLSet = Object.getOwnPropertyDescriptor(Element.prototype, 'innerHTML').set;
    Object.defineProperty(Element.prototype, 'innerHTML', {
        set: function(value) {
            if (typeof value === 'string' && (value.includes(MARKER) || value.includes(TAINT_PREFIX))) {
                console.log('SUKYAN_SINK:innerHTML:' + MARKER);
            }
            return origInnerHTMLSet.call(this, value);
        },
        configurable: true
    });

    // Wrap outerHTML setter
    const origOuterHTMLSet = Object.getOwnPropertyDescriptor(Element.prototype, 'outerHTML').set;
    Object.defineProperty(Element.prototype, 'outerHTML', {
        set: function(value) {
            if (typeof value === 'string' && (value.includes(MARKER) || value.includes(TAINT_PREFIX))) {
                console.log('SUKYAN_SINK:outerHTML:' + MARKER);
            }
            return origOuterHTMLSet.call(this, value);
        },
        configurable: true
    });

    // Wrap document.write
    const origWrite = document.write;
    document.write = function(...args) {
        for (const arg of args) {
            if (typeof arg === 'string' && (arg.includes(MARKER) || arg.includes(TAINT_PREFIX))) {
                console.log('SUKYAN_SINK:document.write:' + MARKER);
            }
        }
        return origWrite.apply(this, args);
    };

    // Wrap document.writeln
    const origWriteln = document.writeln;
    document.writeln = function(...args) {
        for (const arg of args) {
            if (typeof arg === 'string' && (arg.includes(MARKER) || arg.includes(TAINT_PREFIX))) {
                console.log('SUKYAN_SINK:document.writeln:' + MARKER);
            }
        }
        return origWriteln.apply(this, args);
    };

    // Wrap eval
    const origEval = window.eval;
    window.eval = function(code) {
        if (typeof code === 'string' && (code.includes(MARKER) || code.includes(TAINT_PREFIX))) {
            console.log('SUKYAN_SINK:eval:' + MARKER);
        }
        return origEval.call(this, code);
    };

    // Wrap setTimeout with string argument
    const origSetTimeout = window.setTimeout;
    window.setTimeout = function(fn, delay, ...args) {
        if (typeof fn === 'string' && (fn.includes(MARKER) || fn.includes(TAINT_PREFIX))) {
            console.log('SUKYAN_SINK:setTimeout:' + MARKER);
        }
        return origSetTimeout.call(this, fn, delay, ...args);
    };

    // Wrap setInterval with string argument
    const origSetInterval = window.setInterval;
    window.setInterval = function(fn, delay, ...args) {
        if (typeof fn === 'string' && (fn.includes(MARKER) || fn.includes(TAINT_PREFIX))) {
            console.log('SUKYAN_SINK:setInterval:' + MARKER);
        }
        return origSetInterval.call(this, fn, delay, ...args);
    };

    // Wrap location.assign
    const origAssign = location.assign;
    if (origAssign) {
        location.assign = function(url) {
            if (typeof url === 'string' && (url.includes(MARKER) || url.includes(TAINT_PREFIX))) {
                console.log('SUKYAN_SINK:location.assign:' + MARKER);
            }
            return origAssign.call(this, url);
        };
    }

    // Wrap location.replace
    const origReplace = location.replace;
    if (origReplace) {
        location.replace = function(url) {
            if (typeof url === 'string' && (url.includes(MARKER) || url.includes(TAINT_PREFIX))) {
                console.log('SUKYAN_SINK:location.replace:' + MARKER);
            }
            return origReplace.call(this, url);
        };
    }

    window.__sukyanTaintReady = true;
})();
`, marker, payloads.TaintMarkerPrefix)
}
