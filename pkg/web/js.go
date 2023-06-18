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
function getLinks() {
    var array = [];
    if (!document) return array;
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
        console.log(el.attributes.archive)
        if (el.attributes.archive && typeof el.attributes.archive.value === 'string') {
            array.push(absolutePath(el.attributes.archive.value));
        } else if (el.attributes.codebase && typeof el.attributes.codebase.value === 'string') {
            array.push(absolutePath(el.attributes.codebase.value));
        }
    }

    var urlAttrs = ['href', 'src', 'url', 'codebase', 'action', 'ping', 'background', 'cite', 'data', 'dynsrc', 'formaction', 'lowsrc', 'poster', 'longdesc', 'manifest', 'usemap'];

    urlAttrs.forEach(function(attr){
        var elements = document.querySelectorAll('[' + attr + ']');
        for (var i = 0; i < elements.length; i++) {
            var attrValue = elements[i].getAttribute(attr);
            console.log(attrValue)
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
