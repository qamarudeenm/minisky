import { Box, Typography } from '@mui/material';
import { useServices } from '../hooks/useServices';
import ServiceCard from './ServiceCard';
import DataprocManagerDrawer from './DataprocManagerDrawer';
import ServerlessManagerDrawer from './ServerlessManagerDrawer';
import ComputeManagerDrawer from './ComputeManagerDrawer';
import GKEManagerDrawer from './GKEManagerDrawer';
import { useState } from 'react';

export default function ComputePage() {
  const { 
    services, settings, handleStartContainer, handleStopContainer, toggleSetting, handleInstallDependency 
  } = useServices();
  const computeServices = services.filter(s => ['compute', 'gke', 'dataproc', 'serverless', 'cloudfunctions'].includes(s.id));
  const [dataprocDrawerOpen, setDataprocDrawerOpen] = useState(false);
  const [serverlessDrawerOpen, setServerlessDrawerOpen] = useState(false);
  const [computeDrawerOpen, setComputeDrawerOpen] = useState(false);
  const [gkeDrawerOpen, setGkeDrawerOpen] = useState(false);

  const serverlessService = services.find(s => s.id === 'serverless');
  const isBuildpacksEnabled = settings.MINISKY_SERVERLESS_BACKEND === 'buildpacks';
  const missingPack = serverlessService?.missingDeps?.includes('pack') || false;

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
              if (id === 'compute') setComputeDrawerOpen(true);
              if (id === 'dataproc') setDataprocDrawerOpen(true);
              if (id === 'gke') setGkeDrawerOpen(true);
              if (id === 'serverless' || id === 'cloudfunctions') setServerlessDrawerOpen(true);
            }}
            onInstallDependency={handleInstallDependency}
          />
        ))}
      </Box>
      <DataprocManagerDrawer open={dataprocDrawerOpen} onClose={() => setDataprocDrawerOpen(false)} />
      <ServerlessManagerDrawer 
        open={serverlessDrawerOpen} 
        onClose={() => setServerlessDrawerOpen(false)} 
        isBuildpacksEnabled={isBuildpacksEnabled}
        onEnableBuildpacks={() => toggleSetting('MINISKY_SERVERLESS_BACKEND_ENABLED', isBuildpacksEnabled)}
        missingPack={missingPack}
        onInstallPack={() => handleInstallDependency('pack')}
      />
      <ComputeManagerDrawer open={computeDrawerOpen} onClose={() => setComputeDrawerOpen(false)} />
      <GKEManagerDrawer open={gkeDrawerOpen} onClose={() => setGkeDrawerOpen(false)} />
    </Box>
  );
}
