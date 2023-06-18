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
    return array;
}
return getLinks();
}`
