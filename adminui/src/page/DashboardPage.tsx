import { Component } from 'solid-js'
import { AppName } from '../constant'

import stylesCommon from '../common.module.css'

export const DashboardPage: Component = () => {
	return (
		<div
			classList={{
				[stylesCommon.center]: true,
				[stylesCommon.w100]: true,
			}}
		>
			<h1>Welcome to {AppName}</h1>
			<p>
				TODO
			</p>
		</div>
	)
}
