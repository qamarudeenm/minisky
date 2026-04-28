
import { useEffect, useState } from 'react';
import { BrowserRouter as Router, Routes, Route, Link, useLocation } from 'react-router-dom';
import { Box, Drawer, List, ListItemButton, ListItemIcon, ListItemText, Typography } from '@mui/material';
import DashboardIcon from '@mui/icons-material/Dashboard';
import StorageIcon from '@mui/icons-material/Storage';
import ComputeIcon from '@mui/icons-material/Computer';
import DatabaseIcon from '@mui/icons-material/Storage';
import HubIcon from '@mui/icons-material/Hub';
import TerminalIcon from '@mui/icons-material/Terminal';
import BarChartIcon from '@mui/icons-material/BarChart';
import LocalFireDepartmentIcon from '@mui/icons-material/LocalFireDepartment';
import RocketLaunchIcon from '@mui/icons-material/RocketLaunch';
import ScheduleIcon from '@mui/icons-material/Schedule';
import SecurityIcon from '@mui/icons-material/Security';
import Dashboard from './components/Dashboard';
import StoragePage from './components/StoragePage';
import ComputePage from './components/ComputePage';
import DatabasePage from './components/DatabasePage';
import NetworkPage from './components/NetworkPage';
import ProjectSelector from './components/ProjectSelector';
import LogExplorer from './components/LogExplorer';
import MonitoringPage from './components/MonitoringPage';
import FirebasePage from './components/FirebasePage';
import AppEnginePage from './components/AppEnginePage';
import MemorystorePage from './components/MemorystorePage';
import TasksAndSchedulingPage from './components/TasksAndSchedulingPage';
import SecurityPage from './components/SecurityPage';

const DRAWER_WIDTH = 280;

const NAV_ITEMS = [
  { to: '/',           label: 'System Diagnostics',       icon: <DashboardIcon /> },
  { to: '/compute',   label: 'Compute Engine Instances',  icon: <ComputeIcon /> },
  { to: '/storage',   label: 'Data Storage Buckets',      icon: <StorageIcon /> },
  { to: '/database',  label: 'Database Topology',         icon: <DatabaseIcon /> },
  { to: '/network',   label: 'Networking',                icon: <HubIcon /> },
  { to: '/firebase',  label: 'Firebase Services',         icon: <LocalFireDepartmentIcon /> },
  { to: '/appengine', label: 'App Engine',                icon: <RocketLaunchIcon /> },
  { to: '/security',  label: 'Security & Identity',       icon: <SecurityIcon /> },
  { to: '/memorystore', label: 'Memorystore',             icon: <StorageIcon /> },
  { to: '/tasks',       label: 'Tasks & Scheduling',     icon: <ScheduleIcon /> },
];

function NavItem({ to, label, icon }: { to: string; label: string; icon: React.ReactNode }) {
  const { pathname } = useLocation();
  const active = pathname === to;
  return (
    <ListItemButton
      component={Link}
      to={to}
      sx={{
        borderRadius: '8px', mb: 1,
        backgroundColor: active ? '#e8f0fe' : 'transparent',
        '&:hover': { backgroundColor: active ? '#e8f0fe' : '#f1f3f4' }
      }}
    >
      <ListItemIcon sx={{ color: active ? '#1a73e8' : '#5f6368', minWidth: 40 }}>{icon}</ListItemIcon>
      <ListItemText primary={label} sx={{ color: active ? '#1a73e8' : '#3c4043', '& span': { fontWeight: active ? 500 : 400 } }} />
    </ListItemButton>
  );
}

function NavigationContent() {
  const { pathname } = useLocation();
  const isLogging = pathname === '/logging';
  const isMonitoring = pathname === '/monitoring';
  const [version, setVersion] = useState('...');

  useEffect(() => {
    fetch('/api/system/info')
      .then(res => res.json())
      .then(data => setVersion(data.version))
      .catch(err => console.error('Failed to fetch version:', err));
  }, []);

  return (
    <Box sx={{ display: 'flex', minHeight: '100vh' }}>
      {/* Sidebar */}
      <Drawer
        variant="permanent"
        sx={{
          width: DRAWER_WIDTH, flexShrink: 0,
          '& .MuiDrawer-paper': { width: DRAWER_WIDTH, boxSizing: 'border-box' },
        }}
      >
        {/* Logo */}
        <Box sx={{ p: 4, display: 'flex', alignItems: 'center', gap: 2, borderBottom: '1px solid #dadce0' }}>
          <img src="/minisky_logo.png" alt="MiniSky Logo" style={{ width: 36, height: 36, objectFit: 'contain' }} />
          <Typography variant="h6" sx={{ letterSpacing: '0.01em', fontWeight: 500, color: '#3c4043' }}>
            MiniSky v{version}
          </Typography>
        </Box>

        {/* Nav */}
        <List sx={{ px: 2, mt: 2 }}>
          {NAV_ITEMS.map(n => <NavItem key={n.to} {...n} />)}

          {/* Divider before Operations section */}
          <Box sx={{ my: 1.5, mx: 1, borderTop: '1px solid #e0e0e0' }} />
          <Typography variant="caption" sx={{ px: 1, color: '#9aa0a6', fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', fontSize: '0.65rem' }}>
            Operations
          </Typography>

          {/* Cloud Logging */}
          <ListItemButton
            component={Link}
            to="/logging"
            sx={{
              borderRadius: '8px', mt: 1,
              backgroundColor: isLogging ? '#e8f0fe' : 'transparent',
              '&:hover': { backgroundColor: isLogging ? '#e8f0fe' : '#f1f3f4' }
            }}
          >
            <ListItemIcon sx={{ color: isLogging ? '#1a73e8' : '#5f6368', minWidth: 40 }}>
              <TerminalIcon />
            </ListItemIcon>
            <ListItemText
              primary="Cloud Logging"
              sx={{ color: isLogging ? '#1a73e8' : '#3c4043', '& span': { fontWeight: isLogging ? 500 : 400 } }}
            />
          </ListItemButton>

          {/* Cloud Monitoring */}
          <ListItemButton
            component={Link}
            to="/monitoring"
            sx={{
              borderRadius: '8px', mt: 0.5,
              backgroundColor: isMonitoring ? '#e8f0fe' : 'transparent',
              '&:hover': { backgroundColor: isMonitoring ? '#e8f0fe' : '#f1f3f4' }
            }}
          >
            <ListItemIcon sx={{ color: isMonitoring ? '#1a73e8' : '#5f6368', minWidth: 40 }}>
              <BarChartIcon />
            </ListItemIcon>
            <ListItemText
              primary="Cloud Monitoring"
              sx={{ color: isMonitoring ? '#1a73e8' : '#3c4043', '& span': { fontWeight: isMonitoring ? 500 : 400 } }}
            />
          </ListItemButton>
        </List>
      </Drawer>

      {/* Main content */}
      <Box component="main" sx={{
        flexGrow: 1,
        // Log Explorer gets full height with no padding
        ...(isLogging || isMonitoring ? { p: 0, height: '100vh', overflow: 'hidden', display: 'flex', flexDirection: 'column' }
                                          : { p: 6, position: 'relative' })
      }}>
        {!isLogging && !isMonitoring && <ProjectSelector />}
        <Routes>
          <Route path="/"         element={<Dashboard />} />
          <Route path="/compute"  element={<ComputePage />} />
          <Route path="/storage"  element={<StoragePage />} />
          <Route path="/database" element={<DatabasePage />} />
          <Route path="/network"  element={<NetworkPage />} />
          <Route path="/logging"  element={<LogExplorer />} />
          <Route path="/monitoring" element={<MonitoringPage />} />
          <Route path="/firebase"   element={<FirebasePage />} />
          <Route path="/appengine" element={<AppEnginePage />} />
          <Route path="/security"  element={<SecurityPage />} />
          <Route path="/memorystore" element={<MemorystorePage />} />
          <Route path="/tasks" element={<TasksAndSchedulingPage />} />
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
