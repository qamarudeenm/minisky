import { Box, Typography } from '@mui/material';
import { useServices } from '../hooks/useServices';
import ServiceCard from './ServiceCard';
import IamManagerDrawer from './IamManagerDrawer';
import SecretManagerDrawer from './SecretManagerDrawer';
import CloudKmsDrawer from './CloudKmsDrawer';
import { useProjectContext } from '../contexts/ProjectContext';
import { useState } from 'react';

export default function SecurityPage() {
  const { activeProject } = useProjectContext();
  const { 
    services, settings, handleStartContainer, handleStopContainer, toggleSetting, handleInstallDependency 
  } = useServices();
  const securityServices = services.filter(s => ['iam', 'secretmanager', 'cloudkms'].includes(s.id));
  const [iamOpen, setIamOpen] = useState(false);
  const [secretManagerOpen, setSecretManagerOpen] = useState(false);
  const [kmsOpen, setKmsOpen] = useState(false);

  const handleManage = (id: string) => {
    if (id === 'iam') setIamOpen(true);
    if (id === 'secretmanager') setSecretManagerOpen(true);
    if (id === 'cloudkms') setKmsOpen(true);
  };

  return (
    <Box sx={{ animation: 'fadeIn 0.3s ease-out' }}>
      <Typography variant="h4" sx={{ mb: 4, fontWeight: 500 }}>Security &amp; Identity</Typography>
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(3, 1fr)' }, gap: 4 }}>
        {securityServices.map((s, idx) => (
          <ServiceCard 
            key={s.id} 
            service={s} 
            idx={idx} 
            settings={settings}
            onStartContainer={(id) => handleStartContainer(id, activeProject)}
            onStopContainer={handleStopContainer}
            onToggleSetting={toggleSetting}
            onManage={handleManage}
            onInstallDependency={handleInstallDependency}
          />
        ))}
      </Box>
      <IamManagerDrawer open={iamOpen} onClose={() => setIamOpen(false)} />
      <SecretManagerDrawer open={secretManagerOpen} onClose={() => setSecretManagerOpen(false)} />
      <CloudKmsDrawer open={kmsOpen} onClose={() => setKmsOpen(false)} />
    </Box>
  );
}
