import { Component, ErrorBoundary } from 'solid-js'
import { RouteDefinition, Router } from '@solidjs/router'
import { Layout } from './layout/Layout'
import { DashboardPage } from './page/DashboardPage'
import { NotFoundPage } from './page/NotFoundPage'
import { CreateRoomPage } from './page/CreateRoomPage'
import { RoomPage } from './page/RoomPage'
import {
	GlobalSearchFiltersPage,
	RoomSearchFiltersPage,
} from './page/SearchFiltersPage'

const App: Component = () => {
	const routes: RouteDefinition[] = [
		{
			path: '/',
			component: DashboardPage,
		},
		{
			path: '/createroom',
			component: CreateRoomPage,
		},
		{
			path: '/room/:name',
			component: RoomPage,
		},
		{
			path: '/room/:name/filters',
			component: RoomSearchFiltersPage,
		},
		{
			path: '/filters',
			component: GlobalSearchFiltersPage,
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
					fallback={<h1>Fatal error in admin UI, check console</h1>}
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
