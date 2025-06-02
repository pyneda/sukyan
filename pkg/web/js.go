package web

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
        var params = new URLSearchParams();
        
        for (var input of form.querySelectorAll('input, select, textarea')) {
            var name = input.name;
            var value = '';
            
            if (!name) continue;
            
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
        
        var baseUrl = absolutePath(action);
        var queryString = params.toString();
        
        if (queryString) {
            var separator = baseUrl.includes('?') ? '&' : '?';
            formUrls.push(baseUrl + separator + queryString);
        } else {
            formUrls.push(baseUrl);
        }
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
        let url = new URL(item);
        url.hash = '';
        return url.href;
    });

    return [...new Set(array)];
}
return getLinks();
}`
