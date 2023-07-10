package passive

import (
	"strings"
)

// Most patterns taken from arachni:
// https://github.com/Arachni/arachni/blob/master/components/checks/active/xpath_injection/errors.txt

var xpathErrors = []string{
	"XPathEvalError",
	"xmlXPathEval: evaluation failed",
	"SimpleXMLElement::xpath()",
	"XPathException",
	"MS.Internal.Xml",
	"Unknown error in XPath",
	"org.apache.xpath.XPath",
	"A closing bracket expected in",
	"An operand in Union Expression does not produce a node-set",
	"Cannot convert expression to a number",
	"Document Axis does not allow any context Location Steps",
	"Empty Path Expression",
	"Empty Relative Location Path",
	"Empty Union Expression",
	"Expected ')' in",
	"Expected node test or name specification after axis operator",
	"Incompatible XPath key",
	"Incorrect Variable Binding",
	"libxml2 library function failed",
	"xmlsec library function",
	"error '80004005'",
	"A document must contain exactly one root element",
	"Expression must evaluate to a node-set",
	"Expected token ']'",
	"<p>msxml4.dll</font>",
	"<p>msxml3.dll</font>",
	"Invalid predicate",
	"Unexpected end of expression",
	"Invalid number of arguments for function",
	"Unrecognized node type",
	"Expected whitespace in expression",
	"Expected operator in expression",
	"Unexpected XPath character",
	"Unexpected token in XPath expression",
	"Unmatched closing parenthesis",
	"Invalid character in XPath expression",
}

func SearchXPathErrors(text string) string {
	for _, pattern := range xpathErrors {
		if strings.Contains(text, pattern) {
			return pattern
		}
	}
	return ""
}
