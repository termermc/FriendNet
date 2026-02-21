/**
 * Makes a URL to a shared file.
 * @param base The file server base URL.
 * @param serverUuid The server UUID.
 * @param username The peer's username.
 * @param path The path to the file.
 * @param download Whether to download the file instead of viewing it.
 */
export function makeFileUrl(
	base: string,
	serverUuid: string,
	username: string,
	path: string,
	download: boolean,
): string {
	if (path.startsWith('/')) {
		path = path.substring(1)
	}

	return `${base}/${serverUuid}/${username}/${path}${download ? '?download=1' : ''}`
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
export function normalizePath(path: string): { path: string, segments: string[] } {
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

export function makeBrowsePath(serverUuid: string, username: string, path: string): string {
	const { path: normPath } = normalizePath(path)
	return `/server/${serverUuid}/browse/${username}${normPath}`
}
