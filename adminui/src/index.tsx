/* @refresh reload */
import { render } from 'solid-js/web'
import './styles.css'

import { AppLoader } from './AppLoader'

const root = document.getElementById('root')

render(() => <AppLoader />, root!)
