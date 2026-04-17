import { Box, Typography } from '@mui/material';
import { useServices } from '../hooks/useServices';
import ServiceCard from './ServiceCard';
import DataprocManagerDrawer from './DataprocManagerDrawer';
import { useState } from 'react';

export default function ComputePage() {
  const { services, settings, handleStartContainer, handleStopContainer, toggleSetting } = useServices();
  const computeServices = services.filter(s => ['compute', 'gke', 'dataproc', 'serverless'].includes(s.id));
  const [dataprocDrawerOpen, setDataprocDrawerOpen] = useState(false);

  return (
    <Box sx={{ animation: 'fadeIn 0.3s ease-out' }}>
      <Typography variant="h4" sx={{ mb: 4, fontWeight: 500 }}>Compute Engine Instances</Typography>
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(3, 1fr)' }, gap: 4 }}>
        {computeServices.map((s, idx) => (
          <ServiceCard 
            key={s.id} 
            service={s} 
            idx={idx} 
            settings={settings}
            onStartContainer={handleStartContainer}
            onStopContainer={handleStopContainer}
            onToggleSetting={toggleSetting}
            onManage={(id) => {
              if (id === 'dataproc') setDataprocDrawerOpen(true);
            }}
          />
        ))}
      </Box>
      <DataprocManagerDrawer open={dataprocDrawerOpen} onClose={() => setDataprocDrawerOpen(false)} />
    </Box>
  );
}
