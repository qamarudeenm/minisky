import { useState } from 'react';
import { Box, Typography, Button } from '@mui/material';
import RocketLaunchIcon from '@mui/icons-material/RocketLaunch';
import { useServices } from '../hooks/useServices';
import ServiceCard from './ServiceCard';
import AppEngineManagerDrawer from './AppEngineManagerDrawer';

export default function AppEnginePage() {
  const {
    services, settings, handleStartContainer, handleStopContainer,
    toggleSetting, handleInstallDependency,
  } = useServices();
  const [drawerOpen, setDrawerOpen] = useState(false);

  const aeService = services.find(s => s.id === 'appengine');
  const displayServices = aeService ? [aeService] : [];

  return (
    <Box sx={{ animation: 'fadeIn 0.3s ease-out', p: 4 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 4 }}>
        <Box>
          <Typography variant="h4" sx={{ fontWeight: 500 }}>App Engine</Typography>
          <Typography variant="body2" sx={{ color: '#5f6368', mt: 0.5 }}>
            Deploy and scale web applications without managing infrastructure.
          </Typography>
        </Box>
        <Button
          variant="contained"
          startIcon={<RocketLaunchIcon />}
          onClick={() => setDrawerOpen(true)}
          sx={{ fontWeight: 600 }}
        >
          Open Console
        </Button>
      </Box>

      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(3, 1fr)' }, gap: 4 }}>
        {displayServices.map((s, idx) => (
          <ServiceCard
            key={s.id}
            service={s}
            idx={idx}
            settings={settings}
            onStartContainer={handleStartContainer}
            onStopContainer={handleStopContainer}
            onToggleSetting={toggleSetting}
            onManage={() => setDrawerOpen(true)}
            onInstallDependency={handleInstallDependency}
          />
        ))}
      </Box>

      <AppEngineManagerDrawer open={drawerOpen} onClose={() => setDrawerOpen(false)} />

      <style>{`
        @keyframes fadeIn {
          from { opacity: 0; transform: translateY(10px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </Box>
  );
}
