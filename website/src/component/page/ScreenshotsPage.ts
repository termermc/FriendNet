import { html } from 'wunphile'
import type { Component } from 'wunphile'
import { BaseLayout } from '../BaseLayout.ts'

/**
 * The screenshots page.
 */
export const ScreenshotsPage: Component<void, void> = () => {
	return BaseLayout(
		{
			title: 'Screenshots',
			stylesheets: ['/css/home.css'],
		},
		html`
			<div class="home">
                <h1>Screenshots</h1>
				<div class="home-content screenshots">
					<h1>Searching</h1>
					<img src="/asset/screenshot/search.png" alt="search" />
					<br/>
					<img src="/asset/screenshot/search-hakken.png" alt="search hakken" />
					
					<h1>Browsing</h1>
					<img src="/asset/screenshot/browse-preview.png" alt="browse preview" />
					<br/>
					<img src="/asset/screenshot/browse-shares.png" alt="browse shares" />
					
					<h1>Managing Shares</h1>
					<img src="/asset/screenshot/manage-shares.png" alt="manage shares" />
					
					<h1>Custom Profiles</h1>
					<img src="/asset/screenshot/custom-profile.png" alt="custom profile" />
				</div>
			</div>
		`,
	)
}
