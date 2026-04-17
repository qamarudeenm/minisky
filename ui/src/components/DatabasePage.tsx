import { Box, Typography } from '@mui/material';
import { useServices } from '../hooks/useServices';
import ServiceCard from './ServiceCard';
import FirestoreManagerDrawer from './FirestoreManagerDrawer';
import BigQueryManagerDrawer from './BigQueryManagerDrawer';
import CloudSqlManagerDrawer from './CloudSqlManagerDrawer';
import { useState } from 'react';

export default function DatabasePage() {
  const { services, settings, handleStartContainer, handleStopContainer, toggleSetting } = useServices();
  const dbServices = services.filter(s => ['firestore', 'bigquery', 'sqladmin'].includes(s.id));
  const [firestoreOpen, setFirestoreOpen] = useState(false);
  const [bigqueryOpen, setBigqueryOpen] = useState(false);
  const [cloudSqlOpen, setCloudSqlOpen] = useState(false);

  const handleManage = (id: string) => {
    if (id === 'firestore') setFirestoreOpen(true);
    if (id === 'bigquery') setBigqueryOpen(true);
    if (id === 'sqladmin') setCloudSqlOpen(true);
  };

  return (
    <Box sx={{ animation: 'fadeIn 0.3s ease-out' }}>
      <Typography variant="h4" sx={{ mb: 4, fontWeight: 500 }}>Database Topology</Typography>
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(3, 1fr)' }, gap: 4 }}>
        {dbServices.map((s, idx) => (
          <ServiceCard 
            key={s.id} 
            service={s} 
            idx={idx} 
            settings={settings}
            onStartContainer={handleStartContainer}
            onStopContainer={handleStopContainer}
            onToggleSetting={toggleSetting}
            onManage={handleManage}
          />
        ))}
      </Box>
      <FirestoreManagerDrawer open={firestoreOpen} onClose={() => setFirestoreOpen(false)} />
      <BigQueryManagerDrawer open={bigqueryOpen} onClose={() => setBigqueryOpen(false)} />
      <CloudSqlManagerDrawer open={cloudSqlOpen} onClose={() => setCloudSqlOpen(false)} />
    </Box>
  );
}
