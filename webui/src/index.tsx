/* @refresh reload */
import { render } from 'solid-js/web'
import 'solid-devtools'
import './styles.css'

import App from './App'
import AuthError from './AuthError'

const root = document.getElementById('root')

if (import.meta.env.DEV && !(root instanceof HTMLElement)) {
	throw new Error(
		'Root element not found. Did you forget to add it to your index.html? Or maybe the id attribute got misspelled?',
	)
}

const tokenKey = 'friendnet.bearerToken'
const url = new URL(window.location.href)
const queryToken = url.searchParams.get('bearerToken')?.trim() ?? ''
let bearerToken = ''

if (queryToken) {
	localStorage.setItem(tokenKey, queryToken)
	url.searchParams.delete('bearerToken')
	window.history.replaceState({}, document.title, url.toString())
	bearerToken = queryToken
} else {
	bearerToken = localStorage.getItem(tokenKey)?.trim() ?? ''
}

render(() => (bearerToken ? <App bearerToken={bearerToken} /> : <AuthError />), root!)
