export {}

declare global {
	interface Window {
		/**
		 * If true, the app is running in a native-connected webview.
		 */
		__isNative: true | undefined
	}
}
