import { Component, JSX } from 'solid-js'
import { sleep } from './util'
import { TransfersOptionId } from './constant'
import { useGlobalState } from './ctx'

type QueueButtonProps = JSX.HTMLAttributes<HTMLSpanElement> & {
	serverUuid: string
	peerUsername: string
	filePath: string

	children?: string
}

/**
 * QueueButton is a button that queues a file for download.
 */
export const QueueButton: Component<QueueButtonProps> = (props) => {
	const state = useGlobalState()

	const doQueue = (e: MouseEvent) => {
		state.transfer.queue(
			props.serverUuid,
			props.peerUsername,
			props.filePath,
		).catch(err => {
			console.error('failed to queue file:', err)
			alert('Failed to queue file, check console for details')
		})

		; // noinspection ES6MissingAwait
		(async () => {
			const xferElem = document.getElementById(TransfersOptionId)!
			const rect = xferElem.getBoundingClientRect()

			// Do a REALLY COOL animation.
			const elem = document.createElement('div')
			elem.innerText = '📁'
			elem.style.pointerEvents = 'none'
			elem.style.zIndex = '1000'
			elem.style.fontSize = '2rem'
			elem.style.position = 'fixed'
			elem.style.left = e.clientX + 'px'
			elem.style.top = e.clientY + 'px'
			elem.style.transition = 'top 0.5s ease-in-out, left 0.5s ease-in-out, transform 0.25s ease-in-out, opacity 0.5s ease-in-out'
			elem.style.transform = 'scale(0.5)'
			elem.style.transformOrigin = 'center center'

			document.body.appendChild(elem)

			await sleep(16)
			elem.style.left = (rect.left + (rect.width / 2)) + 'px'
			elem.style.top = rect.top + 'px'

			elem.style.transform = 'scale(1)'

			await sleep(250)

			elem.style.transform = 'scale(0.5)'

			await sleep(250)

			document.body.removeChild(elem)
		})()
	}

	return (
		<span
			style="cursor:pointer"
			{...props}
			onClick={doQueue}
		>
			💾
			{props.children ? ' ' + props.children : undefined}
		</span>
	)
}
