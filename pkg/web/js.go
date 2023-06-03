package web

const GetLinks = `getLinks();
function absolutePath(href) {
    try {
        var link = document.createElement("a");
        link.href = href;
        return link.href;
    } catch (error) {}
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
        }
    }
    return array;
}`
