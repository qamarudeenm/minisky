import { useState } from 'react';
import { Box, Typography } from '@mui/material';
import { useServices } from '../hooks/useServices';
import ServiceCard from './ServiceCard';
import StorageManagerDrawer from './StorageManagerDrawer';
import PubSubManagerDrawer from './PubSubManagerDrawer';
export default function StoragePage() {
  const { services, settings, handleStartContainer, handleStopContainer, toggleSetting } = useServices();
  const storageServices = services.filter(s => ['storage', 'pubsub'].includes(s.id));
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [pubSubOpen, setPubSubOpen] = useState(false);

  return (
    <Box sx={{ animation: 'fadeIn 0.3s ease-out' }}>
      <Typography variant="h4" sx={{ mb: 4, fontWeight: 500 }}>Data Storage Buckets</Typography>
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(3, 1fr)' }, gap: 4 }}>
        {storageServices.map((s, idx) => (
          <ServiceCard 
            key={s.id} 
            service={s} 
            idx={idx} 
            settings={settings}
            onStartContainer={handleStartContainer}
            onStopContainer={handleStopContainer}
            onToggleSetting={toggleSetting}
            onManage={(id) => {
              if (id === 'storage') setDrawerOpen(true);
              if (id === 'pubsub') setPubSubOpen(true);
            }}
          />
        ))}
      </Box>

      <StorageManagerDrawer open={drawerOpen} onClose={() => setDrawerOpen(false)} />
      <PubSubManagerDrawer open={pubSubOpen} onClose={() => setPubSubOpen(false)} />
    </Box>
  );
}
