import { Box, Typography, Button } from '@mui/material';
import StorageIcon from '@mui/icons-material/Storage';
import { useServices } from '../hooks/useServices';
import ServiceCard from './ServiceCard';
import { useProjectContext } from '../contexts/ProjectContext';
import { useState } from 'react';
import FirebaseManagerDrawer from './FirebaseManagerDrawer';

export default function FirebasePage() {
  const { activeProject } = useProjectContext();
  const { 
    services, settings, handleStartContainer, handleStopContainer, toggleSetting, handleInstallDependency 
  } = useServices();
  const [drawerOpen, setDrawerOpen] = useState(false);

  const firebaseServices = services.filter(s => s.id.startsWith('firebase-'));

  return (
    <Box sx={{ animation: 'fadeIn 0.3s ease-out', p: 4 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 4 }}>
        <Typography variant="h4" sx={{ fontWeight: 500 }}>Firebase Services</Typography>
        <Button 
          variant="contained" 
          color="warning" 
          startIcon={<StorageIcon />} 
          onClick={() => setDrawerOpen(true)}
          sx={{ fontWeight: 600 }}
        >
          Data Explorer
        </Button>
      </Box>

      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(3, 1fr)' }, gap: 4 }}>
        {firebaseServices.map((s, idx) => (
          <ServiceCard 
            key={s.id} 
            service={s} 
            idx={idx} 
            settings={settings}
            onStartContainer={(id) => handleStartContainer(id, activeProject)}
            onStopContainer={handleStopContainer}
            onToggleSetting={toggleSetting}
            onManage={() => setDrawerOpen(true)}
            onInstallDependency={handleInstallDependency}
          />
        ))}
      </Box>

      <FirebaseManagerDrawer open={drawerOpen} onClose={() => setDrawerOpen(false)} />

      <style>{`
        @keyframes fadeIn {
          from { opacity: 0; transform: translateY(10px); }
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </Box>
  );
}
