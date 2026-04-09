import { html } from 'wunphile'
import type { Component } from 'wunphile'
import { BaseLayout } from '../BaseLayout.ts'
import config from '../../../config.ts'

/**
 * The donation page.
 */
export const DonatePage: Component<void, void> = () => {
	return BaseLayout(
		{
			title: '❤️ Donate',
			stylesheets: ['/css/home.css?v=' + config.buildTimestamp],
		},
		html`
			<div class="home">
				<h1>Donate</h1>
				<div class="home-content donate">
					<p>
						FriendNet is free software, both free as in freedom and free as in beer.
						You do not need to pay anything to use FriendNet, but if you like it, I would appreciate a donation.
                    </p>
					<p>
						You can donate via:
						<ul>
							<li><a href="https://github.com/sponsors/termermc">GitHub Sponsors</a></li>
							<li>Monero/XMR: <code>${config.moneroAddress}</code></li>
						</ul>
					</p>
					<p>
						If you would like to be listed below, please <a href="https://termer.net/">contact me</a> and
						send your name and a link.
					</p>
					
					<h2>Thanks To:</h2>
					<ul>
						<li><a href="https://arisuchan.xyz/">arisuchan.xyz</a></li>
						<li><a href="https://symlinx.net/dacctal/">Dacctal</a></li>
						<li>8chan.moe/t/ anon</li>
					</ul>
				</div>
			</div>
		`,
	)
}
