import { Component, ErrorBoundary } from 'solid-js'
import { Router } from '@solidjs/router'
import { Layout } from './layout/Layout'

const App: Component = () => {
	return (
		<Router
			root={(props) => (
				<ErrorBoundary
					fallback={<h1>Fatal error in web UI, check console</h1>}
				>
					<Layout>{props.children}</Layout>
				</ErrorBoundary>
			)}
		></Router>
	)
}

export default App
