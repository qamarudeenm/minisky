import { createTheme } from '@mui/material/styles';

// Google Cloud Console Authentic Light Theme
export const googleTheme = createTheme({
  palette: {
    mode: 'light',
    primary: {
      main: '#1a73e8', // Official Google Blue
      dark: '#174ea6',
    },
    secondary: {
      main: '#d93025', // Official Google Red
    },
    background: {
      default: '#f8f9fa',
      paper: '#ffffff',
    },
    text: {
      primary: '#202124',
      secondary: '#5f6368',
    },
    divider: '#dadce0',
  },
  typography: {
    fontFamily: '"Roboto", "Arial", sans-serif',
    h1: { fontWeight: 400 },
    h4: { fontWeight: 400, color: '#202124' },
    h6: { fontWeight: 500, fontSize: '1.25rem', color: '#202124' },
    button: { textTransform: 'none', fontWeight: 500, letterSpacing: '0.25px' },
  },
  shape: {
    borderRadius: 4, // GCP uses tight radiuses
  },
  components: {
    MuiButton: {
      styleOverrides: {
        root: {
          boxShadow: 'none',
          '&:hover': {
            boxShadow: '0 1px 2px 0 rgba(60,64,67,0.3), 0 1px 3px 1px rgba(60,64,67,0.15)',
            backgroundColor: '#174ea6',
          },
        },
      },
    },
    MuiPaper: {
      styleOverrides: {
        root: {
          border: '1px solid #dadce0',
          boxShadow: 'none',
        },
      },
    },
    MuiDrawer: {
      styleOverrides: {
        paper: {
          backgroundColor: '#ffffff',
          borderRight: '1px solid #dadce0',
        },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          borderRadius: 4,
          fontWeight: 500,
        }
      }
    }
  },
});
