import { html } from 'wunphile'
import type { Component } from 'wunphile'
import { BaseLayout } from '../BaseLayout.ts'
import config from '../../../config.ts'

/**
 * The homepage.
 */
export const HomePage: Component<void, void> = () => {
	return BaseLayout(
		{
			stylesheets: ['/css/home.css?v=' + config.buildTimestamp],
		},
		html`
			<div class="home">
				<div class="home-header">
					<h1>FriendNet</h1>
					<img src="/logo-wall.png" alt="Logo" />
					<h2>is file sharing</h2>
					<h1>for <span class="friends">friends</span></h1>
				</div>
				<div class="home-content">
					<p>
						FriendNet is <b>self-hostable</b>,
						<b>open source</b> file sharing for friends, like a
						mini-<a href="/docs/compared-to/soulseek/">Soulseek</a>. Unlike
						Soulseek and other
						<a
							href="https://en.wikipedia.org/wiki/Peer-to-peer_file_sharing"
							>P2P networks</a
						>,
						<a href="/docs/peering/">port forwarding is optional</a>
						and it works behind symmetric NAT!
					</p>
					<p>
						You can host a private room for your friends or group to
						<a href="/docs/client/managing-shares/"
							>share folders</a
						>
						on their computers, create
						<a href="/docs/client/profiles/">profiles</a>, and
						<a href="/docs/client/searching">search files.</a>
					</p>
					<p>
						For those familiar with BitTorrent, hosting your own
						FriendNet server can be compared to owning a private
						tracker.
					</p>
					<br />
					<br />
					<div class="center">
						<iframe
							width="560"
							height="315"
							src="https://www.youtube-nocookie.com/embed/cD27wskKkPs?si=wq7lDBIcy09-LB1G"
							title="YouTube video player"
							frameborder="0"
							allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share"
							referrerpolicy="strict-origin-when-cross-origin"
							allowfullscreen
						></iframe>
					</div>
					<br />
					<br />
					<div class="center">
						<a href="/download/" class="big-button"
							>Download Now!</a
						>
						<a href="/screenshots/" class="big-button"
							>Screenshots</a
						>
						<a href="/news/" class="big-button">News</a>
						<a href="/docs/" class="big-button">Documentation</a>
					</div>
				</div>
			</div>

			<div class="home-footer">
				Brought to you by <a href="https://termer.net">termer</a> and
				the <a href="/donate/">friends who helped along the way</a>
			</div>
		`,
	)
}
