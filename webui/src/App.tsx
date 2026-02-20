import type { Component } from 'solid-js'
import { Router } from '@solidjs/router'
import { Layout } from './layout/Layout'

const App: Component = () => {
	return <Router root={(props) => <Layout>{props.children}</Layout>}></Router>
}

export default App
