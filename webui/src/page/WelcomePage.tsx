import { Component } from 'solid-js'
import { AppName } from '../constant'
import { A } from '@solidjs/router'

import stylesCommon from '../common.module.css'

export const WelcomePage: Component = () => {
	return (
		<div
			classList={{
				[stylesCommon.center]: true,
				[stylesCommon.w100]: true,
			}}
		>
			<h1>Welcome to {AppName}</h1>
			<p>
				Not connected to any servers yet?{' '}
				<A href="/createserver">Add</A> one!
			</p>
		</div>
	)
}
