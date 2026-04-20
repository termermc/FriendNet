const KEY_AUTO_OPEN_README = 'friendnet.autoOpenReadme'

export function getAutoOpenReadme(): boolean {
    return localStorage.getItem(KEY_AUTO_OPEN_README) === '1'
}

export function setAutoOpenReadme(enabled: boolean): void {
    if (enabled) {
        localStorage.setItem(KEY_AUTO_OPEN_README, '1')
    } else {
        localStorage.removeItem(KEY_AUTO_OPEN_README)
    }
}
