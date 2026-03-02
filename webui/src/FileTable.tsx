import styles from './FileTable.module.css'
import stylesCommon from './common.module.css'

import { Component, For, JSX, Show } from 'solid-js'
import { A } from '@solidjs/router'
import { guessFileCategory, trimStrEllipsis } from './util'
import { FileMeta } from '../pb/clientrpc/v1/rpc_pb'

/**
 * A file to display in {@link FileTable}.
 */
export type TableFile<T = void> = {
	/**
	 * The file's metadata.
	 */
	meta: FileMeta

	/**
	 * Extra data included with the file.
	 */
	data: T
}

export type FileTableProps<T> = {
	/**
	 * Whether files are currently loading.
	 */
	isLoading: boolean

	/**
	 * The error message to display, if any.
	 */
	error?: string

	/**
	 * The href of the parent directory, if any.
	 */
	parentHref?: string

	/**
	 * The files to display.
	 * All items will be rendered, even if {@link isLoading} is true.
	 */
	files: TableFile<T>[]

	/**
	 * Function that is run for each file.
	 * It returns the options necessary to display the file.
	 */
	forItem: (item: TableFile<T>) => {
		/**
		 * Actions markup for the file.
		 */
		actions?: JSX.Element

		/**
		 * Prefix markup for the file.
		 */
		prefix?: JSX.Element
	} & (
		| {
				/**
				 * The file's href.
				 */
				href: string
		  }
		| {
				/**
				 * The function to run when the file is clicked.
				 */
				onClick: () => void
		  }
	)
}

/**
 * FileTable displays a list of files in a table.
 */
export const FileTable = (<T,>(props: FileTableProps<T>) => {
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
					<Show when={props.error}>
						<tr>
							<td colSpan="2" class={stylesCommon.errorMessage}>
								{props.error}
							</td>
						</tr>
					</Show>

					<Show when={props.parentHref}>
						<tr>
							<td>
								<A
									href={props.parentHref!}
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
					<For each={props.files}>
						{(item) => {
							const meta = item.meta

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

							const options = props.forItem(item)

							return (
								<tr>
									{'href' in options ? (
										<td
											title={meta.name}
											class={styles.label}
										>
											<A href={options.href}>
												{options.prefix}
												{label}
											</A>
										</td>
									) : (
										<td
											title={meta.name}
											onClick={options.onClick}
											class={styles.label}
										>
											<span>
												{options.prefix}
												{label}
											</span>
										</td>
									)}
									<td class={styles.actionsTd}>
										<div class={styles.actions}>
											{options.actions}
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
}) satisfies Component<FileTableProps<any>>
