import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { ThemeProvider, CssBaseline } from '@mui/material'
import { googleTheme } from './theme.ts'
import { ProjectProvider } from './contexts/ProjectContext'
import './index.css'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider theme={googleTheme}>
      <CssBaseline />
      <ProjectProvider>
        <App />
      </ProjectProvider>
    </ThemeProvider>
  </StrictMode>,
)
