import { html } from 'wunphile'
import type { Component } from 'wunphile'
import { BaseLayout } from '../BaseLayout.ts'
import config from '../../../config.ts'

/**
 * Public servers page.
 */
export const PublicServersPage: Component<void, void> = () => {
	return BaseLayout(
		{
			title: 'Public Servers',
			stylesheets: ['/css/home.css?v=' + config.buildTimestamp],
		},
		html`
			<div class="home">
				<h1>Public Servers</h1>
				<div class="home-content donate">
					<p>
						These are <b>UNOFFICIAL</b> public servers,
						<b
							>not affiliated with the FriendNet project in any
							way</b
						>.
					</p>
					<p>
						<a href="https://termer.net/">Contact me</a> to list
						yours. The server must accept registrations or have a
						way to apply to join.
					</p>

					<ul>
						<li>
							<a href="https://friendly.st">friendly.st</a> -
							public server
						</li>
					</ul>
				</div>
			</div>
		`,
	)
}
