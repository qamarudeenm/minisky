
import { BrowserRouter as Router, Routes, Route, Link, useLocation } from 'react-router-dom';
import { Box, Drawer, List, ListItemButton, ListItemIcon, ListItemText, Typography } from '@mui/material';
import DashboardIcon from '@mui/icons-material/Dashboard';
import StorageIcon from '@mui/icons-material/Storage';
import ComputeIcon from '@mui/icons-material/Computer';
import DatabaseIcon from '@mui/icons-material/Storage';
import HubIcon from '@mui/icons-material/Hub';
import Dashboard from './components/Dashboard';

import StoragePage from './components/StoragePage';
import ComputePage from './components/ComputePage';
import DatabasePage from './components/DatabasePage';
import NetworkPage from './components/NetworkPage';
import ProjectSelector from './components/ProjectSelector';

const DRAWER_WIDTH = 280;

function NavigationContent() {
  const location = useLocation();
  const path = location.pathname;

  const isActive = (route: string) => path === route;

  return (
      <Box sx={{ display: 'flex', minHeight: '100vh' }}>
        <Drawer
          variant="permanent"
          sx={{
            width: DRAWER_WIDTH,
            flexShrink: 0,
            '& .MuiDrawer-paper': { 
              width: DRAWER_WIDTH, 
              boxSizing: 'border-box' 
            },
          }}
        >
          <Box sx={{ p: 4, display: 'flex', alignItems: 'center', gap: 2, borderBottom: '1px solid #dadce0' }}>
            <Box 
              sx={{ 
                width: 36, 
                height: 36, 
                borderRadius: '8px', 
                background: '#1a73e8',
                display: 'flex', 
                alignItems: 'center', 
                justifyContent: 'center', 
                color: '#ffffff', 
                fontWeight: 600,
                fontSize: '1.2rem',
                fontFamily: 'monospace'
              }}
            >
              M
            </Box>
            <Typography variant="h6" sx={{ letterSpacing: '0.01em', fontWeight: 500, color: '#3c4043' }}>MiniSky</Typography>
          </Box>
          <List sx={{ px: 2, mt: 2 }}>
            <ListItemButton component={Link} to="/" sx={{ borderRadius: '8px', mb: 1, backgroundColor: isActive('/') ? '#e8f0fe' : 'transparent', '&:hover': { backgroundColor: isActive('/') ? '#e8f0fe' : '#f1f3f4' } }}>
              <ListItemIcon sx={{ color: isActive('/') ? '#1a73e8' : '#5f6368', minWidth: 40 }}><DashboardIcon /></ListItemIcon>
              <ListItemText primary="System Diagnostics" sx={{ color: isActive('/') ? '#1a73e8' : '#3c4043', '& span': { fontWeight: isActive('/') ? 500 : 400 } }} />
            </ListItemButton>
            <ListItemButton component={Link} to="/compute" sx={{ borderRadius: '8px', mb: 1, backgroundColor: isActive('/compute') ? '#e8f0fe' : 'transparent', '&:hover': { backgroundColor: isActive('/compute') ? '#e8f0fe' : '#f1f3f4' } }}>
              <ListItemIcon sx={{ color: isActive('/compute') ? '#1a73e8' : '#5f6368', minWidth: 40 }}><ComputeIcon /></ListItemIcon>
              <ListItemText primary="Compute Engine Instances" sx={{ color: isActive('/compute') ? '#1a73e8' : '#3c4043', '& span': { fontWeight: isActive('/compute') ? 500 : 400 } }} />
            </ListItemButton>
            <ListItemButton component={Link} to="/storage" sx={{ borderRadius: '8px', mb: 1, backgroundColor: isActive('/storage') ? '#e8f0fe' : 'transparent', '&:hover': { backgroundColor: isActive('/storage') ? '#e8f0fe' : '#f1f3f4' } }}>
              <ListItemIcon sx={{ color: isActive('/storage') ? '#1a73e8' : '#5f6368', minWidth: 40 }}><StorageIcon /></ListItemIcon>
              <ListItemText primary="Data Storage Buckets" sx={{ color: isActive('/storage') ? '#1a73e8' : '#3c4043', '& span': { fontWeight: isActive('/storage') ? 500 : 400 } }} />
            </ListItemButton>
            <ListItemButton component={Link} to="/database" sx={{ borderRadius: '8px', mb: 1, backgroundColor: isActive('/database') ? '#e8f0fe' : 'transparent', '&:hover': { backgroundColor: isActive('/database') ? '#e8f0fe' : '#f1f3f4' } }}>
              <ListItemIcon sx={{ color: isActive('/database') ? '#1a73e8' : '#5f6368', minWidth: 40 }}><DatabaseIcon /></ListItemIcon>
              <ListItemText primary="Database Topology" sx={{ color: isActive('/database') ? '#1a73e8' : '#3c4043', '& span': { fontWeight: isActive('/database') ? 500 : 400 } }} />
            </ListItemButton>
            <ListItemButton component={Link} to="/network" sx={{ borderRadius: '8px', mb: 1, backgroundColor: isActive('/network') ? '#e8f0fe' : 'transparent', '&:hover': { backgroundColor: isActive('/network') ? '#e8f0fe' : '#f1f3f4' } }}>
              <ListItemIcon sx={{ color: isActive('/network') ? '#1a73e8' : '#5f6368', minWidth: 40 }}><HubIcon /></ListItemIcon>
              <ListItemText primary="Networking" sx={{ color: isActive('/network') ? '#1a73e8' : '#3c4043', '& span': { fontWeight: isActive('/network') ? 500 : 400 } }} />
            </ListItemButton>
          </List>
        </Drawer>
        
        <Box component="main" sx={{ flexGrow: 1, p: 6, position: 'relative' }}>
          <ProjectSelector />
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/compute" element={<ComputePage />} />
            <Route path="/storage" element={<StoragePage />} />
            <Route path="/database" element={<DatabasePage />} />
            <Route path="/network" element={<NetworkPage />} />
          </Routes>
        </Box>
      </Box>
  );
}

export default function App() {
  return (
    <Router>
      <NavigationContent />
    </Router>
  );
}
