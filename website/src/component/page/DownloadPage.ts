import { html } from 'wunphile'
import type { Component } from 'wunphile'
import { BaseLayout } from '../BaseLayout.ts'
import type { UpdateInfo } from '../../../update.ts'
import config from '../../../config.ts'

type DownloadPageProps = {
	curUpdate: UpdateInfo
}

const releasesUrlRegex =
	/^https:\/\/github\.com\/termermc\/FriendNet\/releases\/tag\/([v.\d]+)\/?$/

/**
 * The download page.
 */
export const DownloadPage: Component<DownloadPageProps, void> = ({
	curUpdate,
}) => {
	const url = curUpdate.url
	const [, releaseTag] = url.match(releasesUrlRegex)

	let dlUrls: Record<string, string>
	if (releaseTag) {
		const baseUrl = `https://github.com/termermc/FriendNet/releases/download/${releaseTag}/friendnet-client`
		const windowsAmd64Suffix = '-windows_amd64.exe'
		const linuxAmd64Suffix = '-linux_amd64'
		const linuxArm64Suffix = '-linux_arm64'
		const debAmd64Suffix = '-linux_amd64.deb'
		const debArm64Suffix = '-linux_arm64.deb'
		// const macosArm64Suffix = '-macos_arm64'

		dlUrls = {
			'Windows (x64)': baseUrl + windowsAmd64Suffix,
			'Linux (x64)': baseUrl + linuxAmd64Suffix,
			'Linux (ARM64)': baseUrl + linuxArm64Suffix,
			// 'MacOS (ARM64)': baseUrl + macosArm64Suffix,
			'MacOS (coming soon, please build from source for now)':
				'/docs/client/compiling/',
			'Debian/Ubuntu (x64)': baseUrl + debAmd64Suffix,
			'Debian/Ubuntu/Raspberry Pi OS (ARM64)': baseUrl + debArm64Suffix,
			'Arch Linux':
				'https://aur.archlinux.org/packages/friendnet-client-bin',
			'Release Page': url,
		}
	} else {
		dlUrls = {
			'All Platforms': url,
		}
	}

	return BaseLayout(
		{
			title: 'Download',
			stylesheets: ['/css/home.css?v=' + config.buildTimestamp],
		},
		html`
			<div class="home">
				<h1>Download FriendNet ${curUpdate.version}</h1>
				<div class="home-content download">
					${curUpdate.description
						? html`
								<h2>Release Notes</h2>
								<pre class="release-notes">
${curUpdate.description}</pre
								>
							`
						: ''}
					<br />
					${Object.entries(dlUrls).map(
						([platform, url]) => html`
							<a href="${url}">${platform}</a>
							<br /><br />
						`,
					)}
					<hr />
					<p>
						Looking for the server download? Check out the
						<a href="/docs/server/setup/">setup guide</a>.
					</p>
				</div>
			</div>
		`,
	)
}
