/**
 * Sleeps for a specified number of milliseconds.
 * @param ms The milliseconds to sleep.
 */
export function sleep(ms: number) {
	return new Promise<void>((res) => setTimeout(res, ms))
}

/**
 * Makes a URL to a shared file.
 * @param base The file server base URL.
 * @param serverUuid The server UUID.
 * @param username The peer's username.
 * @param path The path to the file.
 * @param options Extra options.
 */
export function makeFileUrl(
	base: string,
	serverUuid: string,
	username: string,
	path: string,
	options: {
		download?: boolean
		allowCache?: boolean
		zip?: boolean
	} = {},
): string {
	if (path.startsWith('/')) {
		path = path.substring(1)
	}

	const query = new URLSearchParams()
	if (options.allowCache) {
		query.set('allowCache', '1')
	}
	if (options.download) {
		query.set('download', '1')
	}
	if (options.zip) {
		query.set('zip', '1')
	}

	return `${base}/${serverUuid}/${username}/${escapePathSegments(path)}?${query}`
}

/**
 * Broad file categories.
 */
export type FileCategory = 'image' | 'video' | 'audio' | 'text' | 'other'

/**
 * Guesses a file's category based on its filename.
 * @param filename The file's name.
 * @returns The guessed category.
 */
export function guessFileCategory(filename: string): FileCategory {
	const dotIdx = filename.lastIndexOf('.')
	let ext: string
	if (dotIdx === -1) {
		return 'other'
	} else {
		ext = filename.substring(dotIdx + 1).toLowerCase()
	}

	// I did not include "ts" in the list because it could be either
	// a video or a TypeScript file. If we try to preview it, and we
	// were wrong about the category, it will show garbage or fail.

	switch (ext) {
		case 'jpg':
		case 'jpeg':
		case 'png':
		case 'bmp':
		case 'gif':
		case 'svg':
		case 'webp':
		case 'ico':
		case 'tiff':
		case 'psd':
			return 'image'
		case 'mp4':
		case 'webm':
		case 'mkv':
		case 'flv':
		case 'ogv':
		case 'avi':
		case 'wmv':
		case 'm4v':
		case '3gp':
		case 'mov':
		case 'mts':
		case 'm2ts':
		case 'vob':
		case 'f4v':
			return 'video'
		case 'mp3':
		case 'wav':
		case 'flac':
		case 'aac':
		case 'm4a':
		case 'ogg':
		case 'wma':
		case 'aiff':
		case 'alac':
		case 'ape':
		case 'opus':
			return 'audio'
		case 'txt':
		case 'pdf':
		case 'doc':
		case 'docx':
		case 'rtf':
		case 'odt':
		case 'md':
		case 'tex':
		case 'json':
		case 'xml':
		case 'csv':
		case 'html':
		case 'htm':
		case 'js':
		case 'py':
		case 'java':
		case 'cpp':
		case 'c':
		case 'go':
		case 'rs':
		case 'rb':
		case 'php':
		case 'cs':
		case 'swift':
		case 'kt':
		case 'scala':
		case 'sh':
		case 'bash':
		case 'yaml':
		case 'yml':
		case 'toml':
		case 'ini':
		case 'log':
		case 'cfg':
		case 'conf':
		case 'css':
		case 'scss':
			return 'text'
		default:
			return 'other'
	}
}

/**
 * Normalizes a path, returning the normalized path and the constituent segments.
 * @param path The path to normalize.
 * @returns The normalized path and the constituent segments.
 */
export function normalizePath(path: string): {
	path: string
	segments: string[]
} {
	let pathRes: string
	const pathSegments: string[] = []
	{
		const segments = path.split('/')
		for (const part of segments) {
			if (!part || part === '.') {
				continue
			}

			if (part === '..') {
				pathSegments.pop()
				continue
			}

			pathSegments.push(part)
		}

		pathRes = '/' + pathSegments.join('/')
	}

	return {
		path: pathRes,
		segments: pathSegments,
	}
}

function escapePathSegments(path: string): string {
	return path
		.split('/')
		.map((x) => encodeURIComponent(x))
		.join('/')
}

export function makeBrowsePath(
	serverUuid: string,
	username: string,
	path: string,
): string {
	if (path === '' || path === '/') {
		return `/server/${serverUuid}/browse/${username}`
	}

	const { path: normPath } = normalizePath(path)
	return `/server/${serverUuid}/browse/${username}${escapePathSegments(normPath)}`
}

export function trimStrEllipsis(str: string, len: number): string {
	if (str.length <= len) {
		return str
	}

	if (len <= 3) {
		return '...'.substring(0, len)
	}

	return str.substring(0, len - 3) + '...'
}

/**
 * Takes in a generator and collects it into an array.
 * @param gen The generator.
 * @param limit The maximum number of elements to collect. If undefined, collects all elements.
 * @param preallocate The number of elements to preallocate in the array.
 * @returns The array of values.
 */
export function collect<T>(
	gen: Generator<T>,
	limit: number | undefined = undefined,
	preallocate: number = 0,
): T[] {
	if (limit === 0) {
		return []
	}

	if (preallocate <= 0) {
		const res: T[] = []
		let count = 0
		for (const val of gen) {
			res.push(val)
			count++
			if (count === limit) {
				break
			}
		}
		return res
	}

	const preallocateCount =
		limit == null ? preallocate : Math.min(preallocate, limit)
	const res = new Array<T>(preallocateCount)

	let i = 0
	for (const val of gen) {
		if (i < preallocateCount) {
			res[i] = val
		} else {
			res.push(val)
		}
		i++
		if (i === limit) {
			break
		}
	}

	if (i < preallocateCount) {
		res.length = i
	}

	return res
}
