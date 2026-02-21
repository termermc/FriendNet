import { Component } from 'solid-js'
import { useLocation } from '@solidjs/router'

import stylesCommon from '../common.module.css'

export const NotFoundPage: Component = () => {
	const loc = useLocation()

	return (
		<div
			classList={{
				[stylesCommon.center]: true,
				[stylesCommon.w100]: true,
			}}
		>
			<h1>404 Not Found</h1>
			<p>Page not found: {loc.pathname}</p>
		</div>
	)
}
