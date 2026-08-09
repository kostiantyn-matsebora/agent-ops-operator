import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import '@patternfly/react-core/dist/styles/base.css'
import './theme/theme.css'
import { App } from './App'
import { startThemeSync } from './theme/useTheme'

// Applied BEFORE the first render: doing it in an effect would paint the
// default theme for a frame and flash on every load.
startThemeSync()

// Snapshots are authoritative and the stream invalidates them, so queries do not
// need aggressive polling of their own — retry stays low because a failing
// endpoint should surface its reason rather than be retried into a spinner.
const client = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 5_000 },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
