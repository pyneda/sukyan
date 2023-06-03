package active

// Interesting resources
// - https://book.hacktricks.xyz/pentesting-web/file-upload

// Notes:
// - Should only be launched if a file upload has been detected on the page
// - It should try to guess expected formats
// - Check if non expected formats are allowed
// - If not allowed, check if it can be bypassed
// - Extra checks depending on formats:
// 		- Upload image: can try XSS via image with payload in metadata
