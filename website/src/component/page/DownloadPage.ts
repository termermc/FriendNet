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

	let items: {
		name: string
		url: string
		subtitle?: string
		icons?: string[]
	}[]
	if (releaseTag) {
		const baseUrl = `https://github.com/termermc/FriendNet/releases/download/${releaseTag}/friendnet-client`
		const windowsAmd64Suffix = '-windows_amd64.exe'
		const linuxAmd64Suffix = '-linux_amd64'
		const linuxArm64Suffix = '-linux_arm64'
		const debAmd64Suffix = '-linux_amd64.deb'
		const debArm64Suffix = '-linux_arm64.deb'
		// const macosArm64Suffix = '-macos_arm64'

		items = [
			{
				name: 'Windows (x64)',
				url: baseUrl + windowsAmd64Suffix,
				subtitle: 'Requires Windows 10 or later.',
				icons: ['windows.svg'],
			},
			{
				name: 'MacOS (coming soon)',
				url: '/docs/client/compiling/',
				subtitle: 'Please build from source for now.',
				icons: ['apple.svg'],
			},
			{
				name: 'Linux (x64)',
				url: baseUrl + linuxAmd64Suffix,
				subtitle:
					'Works on all distros. Use if there is no specific package for your distro.',
				icons: ['linux.svg'],
			},
			{
				name: 'Linux (ARM64)',
				url: baseUrl + linuxArm64Suffix,
				subtitle:
					'Works on all distros. Use if there is no specific package for your distro.',
				icons: ['linux.svg'],
			},
			{
				name: 'Debian/Ubuntu (x64)',
				url: baseUrl + debAmd64Suffix,
				subtitle: 'Works on Debian-based and Ubuntu-based distros.',
				icons: ['debian.svg', 'ubuntu.svg'],
			},
			{
				name: 'Debian/Ubuntu (ARM64)',
				url: baseUrl + debArm64Suffix,
				subtitle:
					'Works on Debian-based and Ubuntu-based distros. Use this if you use Raspberry Pi OS.',
				icons: ['debian.svg', 'ubuntu.svg', 'raspberry-pi.svg'],
			},
			{
				name: 'Arch Linux',
				url: 'https://aur.archlinux.org/packages/friendnet-client-bin',
				subtitle:
					'Binary releases are provided by friendnet-client-bin (AUR).',
				icons: ['archlinux.svg'],
			},
			{
				name: 'Release Page',
				url: url,
				icons: ['github.svg'],
			},
		]
	} else {
		items = [
			{
				name: 'All Platforms',
				url: url,
			},
		]
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
								<div
									class="release-notes"
									style="white-space: preserve-breaks"
								>
									${curUpdate.description}
								</div>
							`
						: ''}
					<br />
					<div class="download-items">
						${items.map(
							(item) => html`
								<div class="download-item">
									<a
										href="${item.url}"
										class="download-item-link"
									>
										${item.icons?.map(
											(icon) =>
												html`<img
													src="/asset/icon/${icon}"
													alt="${icon}"
													class="download-item-icon"
												/>`,
										)}
										${item.name}
										<img
											src="/asset/icon/download.svg"
											alt="download"
											class="download-item-download"
										/>
									</a>
									${item.subtitle
										? html`
												<div
													class="download-item-subtitle"
												>
													${item.subtitle}
												</div>
											`
										: ''}
								</div>
							`,
						)}
					</div>
					<hr />
					<p>
						<b>Looking for the server download?</b>
						Check out the
						<a href="/docs/server/setup/">setup guide</a>.
					</p>
				</div>
			</div>
		`,
	)
}
