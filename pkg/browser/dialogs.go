package browser

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// func OverrideJSDialogs(page *rod.Page) error {
// 	script := `() => {
//         const nativeAlert = window.alert;
//         const nativeConfirm = window.confirm;
//         const nativePrompt = window.prompt;

//         window.alert = () => true;
//         alert = () => true;
//         window.confirm = () => true;
//         confirm = () => true;
//         window.prompt = () => true;
//         prompt = () => true;

//         window.addEventListener('dialog', event => {
//             event.preventDefault();
//             return true;
//         }, true);

//         window.onbeforeunload = null;
//         Object.defineProperty(window, 'onbeforeunload', {
//             configurable: false,
//             writable: false,
//             value: null
//         });
//     }`
// 	_, err := page.Eval(script)
// 	return err
// }

func CloseAllJSDialogs(page *rod.Page) error {
	// Handle JavaScript dialogs at protocol level
	go page.EachEvent(func(e *proto.PageJavascriptDialogOpening) (stop bool) {
		_ = proto.PageHandleJavaScriptDialog{
			Accept:     true,
			PromptText: "",
		}.Call(page)
		return true
	})()
	return nil
}
