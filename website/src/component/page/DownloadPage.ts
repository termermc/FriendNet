import { html } from 'wunphile'
import type { Component } from 'wunphile'
import { BaseLayout } from '../BaseLayout.ts'
import type { UpdateInfo } from '../../../update.ts'

type DownloadPageProps = {
    curUpdate: UpdateInfo
}

const releasesUrlRegex = /^https:\/\/github\.com\/termermc\/FriendNet\/releases\/tag\/([v.\d]+)\/?$/

/**
 * The download page.
 */
export const DownloadPage: Component<DownloadPageProps, void> = ({ curUpdate }) => {
    const url = curUpdate.url
    const [, releaseTag] = url.match(releasesUrlRegex)

    let dlUrls: Record<string, string>
    if (releaseTag) {
        const baseUrl = `https://github.com/termermc/FriendNet/releases/download/${releaseTag}/friendnet-client`
        const windowsAmd64Suffix = '-windows_amd64.exe'
        const linuxAmd64Suffix = '-linux_amd64'
        const linuxArm64Suffix = '-linux_arm64'
        const macosArm64Suffix = '-macos_arm64'

        dlUrls = {
            'Windows (x64)': baseUrl + windowsAmd64Suffix,
            'Linux (x64)': baseUrl + linuxAmd64Suffix,
	        'Linux (ARM64)': baseUrl + linuxArm64Suffix,
            'MacOS (Apple Silicon, M1, etc.)': baseUrl + macosArm64Suffix,
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
			stylesheets: ['/css/home.css'],
		},
		html`
			<div class="home">
                <h1>Download FriendNet ${curUpdate.version}</h1>
				<div class="home-content download">
                    ${curUpdate.description
                            ? html`
                                <h2>Release Note</h2>
                                <pre>${curUpdate.description}</pre>
                            `
                            : ''
                    }
                    <br/>
                    ${Object.entries(dlUrls).map(([platform, url]) => (
                        html`
                            <a href="${url}">${platform}</a>
                            <br/><br/>
                        `
                    ))}
                    <hr/>
                    <p>
                        Looking for the server download? Check out the
                        <a href="/docs/server/setup/">setup guide</a>.
                    </p>
				</div>
			</div>
		`,
	)
}
