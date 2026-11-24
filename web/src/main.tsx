import ReactDOM from 'react-dom/client'
import '@arco-design/web-react/dist/css/arco.css'
import './app.css'
import App from './App'

document.body.setAttribute('arco-theme', 'dark')

ReactDOM.createRoot(document.getElementById('root')!).render(<App />)
