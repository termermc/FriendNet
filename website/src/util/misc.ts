const monthNamesShort = [
	'Jan',
	'Feb',
	'Mar',
	'Apr',
	'May',
	'Jun',
	'Jul',
	'Aug',
	'Sep',
	'Oct',
	'Nov',
	'Dec',
]

export function formatDate(date: Date): string {
	const year = date.getFullYear()
	const month = monthNamesShort[date.getMonth()]
	const day = date.getDate().toString().padStart(2, '0')
	return `${month} ${day}, ${year} (UTC)`
}
