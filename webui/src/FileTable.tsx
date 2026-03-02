import { Component, For, Show } from 'solid-js'
import styles from './page/ServerBrowsePage.module.css'
import stylesCommon from './common.module.css'
import { A } from '@solidjs/router'
import { guessFileCategory, makeBrowsePath, makeFileUrl, trimStrEllipsis } from './util'

export type FileTableProps = {
	/**
	 * Whether files are currently loading.
	 */
	isLoading: boolean
}

/**
 * FileTable displays a list of files in a table.
 */
export const FileTable: Component<FileTableProps> = (props) => {
	// TODO Make file browser table code generic enough to use for that page and search.

	return (
		<div class={styles.files}>
			<table>
				<thead>
				<tr>
					<th>File</th>
					<th>Actions</th>
				</tr>
				</thead>
				<tbody>
				<Show when={props.isLoading}>
					<tr>
						<td colSpan="2">Loading...</td>
					</tr>
				</Show>
				<Show when={error()}>
					<tr>
						<td
							colSpan="2"
							class={stylesCommon.errorMessage}
						>
							{error()}
						</td>
					</tr>
				</Show>

				<Show when={pathSegments.length !== 0}>
					<tr>
						<td>
							<A
								href={makeBrowsePath(
									uuid,
									username,
									pathSegments.slice(0, -1).join('/'),
								)}
								title="Up a directory"
								classList={{
									[stylesCommon.w100]: true,
									[stylesCommon.displayInlineBlock]: true,
								}}
							>
								▲ ..
							</A>
						</td>
					</tr>
				</Show>
				<For each={files()}>
					{(meta) => {
						let pth = path === '/' ? '' : path

						const filePath = pth + '/' + meta.name
						const dlUrl = makeFileUrl(
							fsUrl,
							uuid,
							username,
							filePath,
							{
								download: true,
							},
						)
						const nonDlUrl = makeFileUrl(
							fsUrl,
							uuid,
							username,
							filePath,
						)

						let emoji: string
						if (meta.isDir) {
							emoji = '📁'
						} else {
							const cat = guessFileCategory(meta.name)
							switch (cat) {
								case 'text':
									emoji = '📜'
									break
								case 'image':
									emoji = '🖼️'
									break
								case 'video':
									emoji = '🎞️'
									break
								case 'audio':
									emoji = '🎵'
									break
								case 'other':
									emoji = '📄'
									break
							}
						}

						const label = trimStrEllipsis(
							emoji + ' ' + meta.name,
							100,
						)

						return (
							<tr>
								<Show
									when={meta.isDir}
									fallback={
										<td
											title={meta.name}
											onClick={() =>
												state.previewFile(
													uuid,
													username,
													filePath,
												)
											}
											class={styles.label}
										>
											<span>{label}</span>
										</td>
									}
								>
									<td
										title={meta.name}
										class={styles.label}
									>
										<A
											href={makeBrowsePath(
												uuid,
												username,
												filePath,
											)}
										>
											{label}
										</A>
									</td>
								</Show>
								<td class={styles.actionsTd}>
									<div class={styles.actions}>
										<Show when={!meta.isDir}>
											<a
												href={nonDlUrl}
												target="_blank"
											>
												🔗
											</a>
											<a href={dlUrl}>💾</a>
										</Show>
									</div>
								</td>
							</tr>
						)
					}}
				</For>
				</tbody>
			</table>
		</div>
	)
}
