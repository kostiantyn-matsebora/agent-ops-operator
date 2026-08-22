import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClientProvider } from '@tanstack/react-query'
import '@patternfly/react-core/dist/styles/base.css'
import './theme/theme.css'
import { App } from './App'
import { startThemeSync } from './theme/useTheme'
import { createQueryClient } from './api/queryClient'

// Applied BEFORE the first render: doing it in an effect would paint the
// default theme for a frame and flash on every load.
startThemeSync()

// The cache's bounds live in ONE place, with their reasoning — see queryClient.ts.
const client = createQueryClient()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
