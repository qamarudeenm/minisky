import { useState } from 'react';
import { Box, Typography, Button } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import { useServices } from '../hooks/useServices';
import ServiceCard from './ServiceCard';
import MemorystoreManagerDrawer from './MemorystoreManagerDrawer';

export default function MemorystorePage() {
  const {
    services, settings, handleStartContainer, handleStopContainer,
    toggleSetting, handleInstallDependency,
  } = useServices();
  const [drawerOpen, setDrawerOpen] = useState(false);

  const memoService = services.find(s => s.id === 'memorystore');
  const displayServices = memoService ? [memoService] : [];

  return (
    <Box sx={{ animation: 'fadeIn 0.3s ease-out', p: 4 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 4 }}>
        <Box>
          <Typography variant="h4" sx={{ fontWeight: 500 }}>Memorystore</Typography>
          <Typography variant="body2" sx={{ color: '#5f6368', mt: 0.5 }}>
            In-memory data store service for Redis and Memcached.
          </Typography>
        </Box>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          onClick={() => setDrawerOpen(true)}
          sx={{ fontWeight: 600 }}
        >
          Create Instance
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

      <MemorystoreManagerDrawer open={drawerOpen} onClose={() => setDrawerOpen(false)} />

      <style>{`
        @keyframes fadeIn {
          from { opacity: 0; transform: translateY(10px); }
          to   { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </Box>
  );
}
