package web

import "strings"

// EscapeJSString escapes a string for safe use in JavaScript string literals.
// It handles backslashes, quotes, common whitespace, JS line terminators (U+2028, U+2029),
// null bytes, backticks, and template literal interpolation.
func EscapeJSString(s string) string {
	// Escape backslash first (must be first to avoid double-escaping)
	s = strings.ReplaceAll(s, `\`, `\\`)
	// Escape quotes
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	// Escape common whitespace
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	// Escape JavaScript line terminators that are NOT escaped by Go's string escaping
	// U+2028 (Line Separator) and U+2029 (Paragraph Separator) are valid JS line terminators
	// that will break string literals if not escaped
	s = strings.ReplaceAll(s, "\u2028", `\u2028`)
	s = strings.ReplaceAll(s, "\u2029", `\u2029`)
	// Escape null bytes which can cause issues
	s = strings.ReplaceAll(s, "\x00", `\x00`)
	// Escape backticks for template literal contexts
	s = strings.ReplaceAll(s, "`", "\\`")
	// Escape ${} to prevent template literal interpolation
	s = strings.ReplaceAll(s, "${", `\${`)
	return s
}

const GetLinks = `() => {
function absolutePath(href) {
    try {
        var link = document.createElement("a");
        link.href = href;
        return link.href;
    } catch (error) {
        return href;
    }
    
}
function processSrcset(srcset) {
    return srcset.split(',')
        .map(s => s.trim().split(/\s+/)[0])
        .filter(Boolean);
}
function getFormUrls() {
   var formUrls = [];
   var forms = document.querySelectorAll('form');
   
   for (var form of forms) {
       var action = form.action || window.location.href;
       var method = (form.method || 'GET').toUpperCase();
       var hasPasswordField = false;
       var params = new URLSearchParams();
       
       for (var input of form.querySelectorAll('input, select, textarea')) {
           var name = input.name;
           var value = '';
           
           if (!name) continue;
           
           if (input.type === 'password') {
               hasPasswordField = true;
           }
           
           if (input.type === 'checkbox' || input.type === 'radio') {
               if (input.checked) {
                   value = input.value || 'on';
               } else if (input.defaultChecked) {
                   value = input.value || 'on';
               } else {
                   continue;
               }
           } else if (input.type === 'submit' || input.type === 'button') {
               continue;
           } else if (input.tagName === 'SELECT') {
               var selected = input.querySelector('option[selected]') || input.options[0];
               value = selected ? selected.value : '';
           } else {
               value = input.value || input.defaultValue || input.placeholder || '';
           }
           
           params.append(name, value);
       }
       
       if (hasPasswordField && method !== 'GET') {
           continue;
       }
       
       var baseUrl = absolutePath(action);
       var existingUrl = new URL(baseUrl);
       var existingParams = new URLSearchParams(existingUrl.search);
       
       for (var [key, value] of params) {
           existingParams.set(key, value);
       }
       
       var finalUrl = existingUrl.origin + existingUrl.pathname + 
           (existingParams.toString() ? '?' + existingParams.toString() : '');
       
       formUrls.push(finalUrl);
   }
   
   return formUrls;
}
function getLinks() {
    var array = [];
    if (!document) return array;

    try {
        var formUrls = getFormUrls();
        array = array.concat(formUrls);
    } catch (error) {}

    var allElements = document.querySelectorAll("a");
    for (var el of allElements) {
        if (el.href && typeof el.href === 'string') {
            array.push(el.href);
        } else if (el.src && typeof el.src === 'string') {
            var absolute = absolutePath(el.src);
            array.push(absolute);
        } else if (el.ping && typeof el.ping === 'string') {
            var absolute = absolutePath(el.ping);
            array.push(absolute);
        }
    }

    var applets = document.querySelectorAll("applet");
    for (var el of applets) {
        if (el.attributes.archive && typeof el.attributes.archive.value === 'string') {
            array.push(absolutePath(el.attributes.archive.value));
        } else if (el.attributes.codebase && typeof el.attributes.codebase.value === 'string') {
            array.push(absolutePath(el.attributes.codebase.value));
        }
    }

    var urlAttrs = ['href', 'src', 'url', 'codebase', 'action', 'ping', 'background', 'cite', 'data', 'dynsrc', 'formaction', 'lowsrc', 'poster', 'longdesc', 'manifest', 'usemap', 'routerlink', 'ng-href'];

    urlAttrs.forEach(function(attr){
        var elements = document.querySelectorAll('[' + attr + ']');
        for (var i = 0; i < elements.length; i++) {
            var attrValue = elements[i].getAttribute(attr);
            if (attrValue && typeof attrValue === 'string') {
                var absolute = absolutePath(attrValue);
                array.push(absolute);
            }
        }
    });

    var svgElements = document.querySelectorAll('image');
    for (var i = 0; i < svgElements.length; i++) {
        var attrValue = svgElements[i].getAttributeNS('http://www.w3.org/1999/xlink', 'href');
        if (attrValue && typeof attrValue === 'string') {
            var absolute = absolutePath(attrValue);
            array.push(absolute);
        }
    }

    var srcsetElements = document.querySelectorAll('[srcset]');
    for (var i = 0; i < srcsetElements.length; i++) {
        var attrValue = srcsetElements[i].getAttribute('srcset');
        if (attrValue && typeof attrValue === 'string') {
            var urls = processSrcset(attrValue);
            urls.forEach(url => {
                var absolute = absolutePath(url);
                array.push(absolute);
            });
        }
    }


    

    array = array.filter(item => item.startsWith("http")).map(item => {
        try {
            let url = new URL(item);
            url.hash = '';
            return url.href;
        } catch (error) {
            return item;
        }
    });

    return [...new Set(array)];
}
return getLinks();
}`
