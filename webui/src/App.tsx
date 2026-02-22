import { Component, ErrorBoundary } from 'solid-js'
import { RouteDefinition, Router } from '@solidjs/router'
import { Layout } from './layout/Layout'
import { WelcomePage } from './page/WelcomePage'
import { NotFoundPage } from './page/NotFoundPage'
import { CreateServerPage } from './page/CreateServerPage'
import { EditServerPage } from './page/EditServerPage'
import { ServerSharesPage } from './page/ServerSharesPage'
import { ServerBrowsePage } from './page/ServerBrowsePage'
import { makeBrowsePath } from './util'

const App: Component = () => {
	const routes: RouteDefinition[] = [
		{
			path: '/',
			component: WelcomePage,
		},
		{
			path: '/createserver',
			component: CreateServerPage,
		},

		{
			path: '/server/:uuid/edit',
			component: EditServerPage,
		},
		{
			path: '/server/:uuid/shares',
			component: ServerSharesPage,
		},

		{
			path: makeBrowsePath(':uuid', ':username', '*path'),
			component: ServerBrowsePage,
		},

		{
			path: '*404',
			component: NotFoundPage,
		},
	]

	return (
		<Router
			explicitLinks={true}
			root={(props) => (
				<ErrorBoundary
					fallback={<h1>Fatal error in web UI, check console</h1>}
				>
					<Layout>{props.children}</Layout>
				</ErrorBoundary>
			)}
		>
			{routes}
		</Router>
	)
}

export default App
