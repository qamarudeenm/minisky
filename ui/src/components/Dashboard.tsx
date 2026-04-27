import { useState } from 'react';
import { Box, Typography } from '@mui/material';
import { useServices } from '../hooks/useServices';
import ServiceCard from './ServiceCard';
import StorageManagerDrawer from './StorageManagerDrawer';
import IamManagerDrawer from './IamManagerDrawer';
import ComputeManagerDrawer from './ComputeManagerDrawer';
import NetworkManagerDrawer from './NetworkManagerDrawer';
import FirestoreManagerDrawer from './FirestoreManagerDrawer';
import PubSubManagerDrawer from './PubSubManagerDrawer';
import CloudSqlManagerDrawer from './CloudSqlManagerDrawer';
import DataprocManagerDrawer from './DataprocManagerDrawer';
import ServerlessManagerDrawer from './ServerlessManagerDrawer';
import BigQueryManagerDrawer from './BigQueryManagerDrawer';
import BigtableManagerDrawer from './BigtableManagerDrawer';
import GKEManagerDrawer from './GKEManagerDrawer';
import AppEngineManagerDrawer from './AppEngineManagerDrawer';

export default function Dashboard() {
  const { 
    services, settings, handleStartContainer, handleStopContainer, toggleSetting, handleInstallDependency 
  } = useServices();
  const runningServices = services.filter(s => s.status === 'RUNNING');
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [iamDrawerOpen, setIamDrawerOpen] = useState(false);
  const [computeDrawerOpen, setComputeDrawerOpen] = useState(false);
  const [networkDrawerOpen, setNetworkDrawerOpen] = useState(false);
  const [firestoreDrawerOpen, setFirestoreDrawerOpen] = useState(false);
  const [pubsubDrawerOpen, setPubsubDrawerOpen] = useState(false);
  const [cloudSqlDrawerOpen, setCloudSqlDrawerOpen] = useState(false);
  const [dataprocDrawerOpen, setDataprocDrawerOpen] = useState(false);
  const [serverlessDrawerOpen, setServerlessDrawerOpen] = useState(false);
  const [bigQueryDrawerOpen, setBigQueryDrawerOpen] = useState(false);
  const [bigtableDrawerOpen, setBigtableDrawerOpen] = useState(false);
  const [gkeDrawerOpen, setGkeDrawerOpen] = useState(false);
  const [appEngineDrawerOpen, setAppEngineDrawerOpen] = useState(false);

  const serverlessService = services.find(s => s.id === 'serverless');
  const isBuildpacksEnabled = settings.MINISKY_SERVERLESS_BACKEND === 'buildpacks';
  const missingPack = serverlessService?.missingDeps?.includes('pack') || false;

  return (
    <Box sx={{ animation: 'fadeIn 0.5s ease-out' }}>
      <Box sx={{ mb: 6, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
        <Box>
          <Typography variant="h4" sx={{ mb: 1.5, fontWeight: 500 }}>System Diagnostics</Typography>
          <Typography variant="body1" sx={{ color: '#5f6368', maxWidth: 650, lineHeight: 1.5 }}>
            The Minisky Daemon is actively intercepting native Google Cloud SDK requests on <strong style={{color: '#202124'}}>localhost:8080</strong>. Services spin up lazily upon impact.
          </Typography>
        </Box>
      </Box>

      <Typography variant="h6" sx={{ mb: 3, color: 'var(--text-primary)' }}>Currently Active Endpoints</Typography>
      
      {runningServices.length === 0 ? (
        <Box sx={{ p: 4, background: '#f8f9fa', borderRadius: '10px', border: '1px dashed #dadce0', textAlign: 'center' }}>
          <Typography variant="body1" sx={{ color: '#80868b' }}>No active endpoints. Services will spin up automatically when invoked, or you can manually enable them in their specific tabs.</Typography>
        </Box>
      ) : (
        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(3, 1fr)' }, gap: 4 }}>
          {runningServices.map((s, idx) => (
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
                if (id === 'iam') setIamDrawerOpen(true);
                if (id === 'compute') setComputeDrawerOpen(true);
                if (id === 'dns') setNetworkDrawerOpen(true);
                if (id === 'firestore') setFirestoreDrawerOpen(true);
                if (id === 'pubsub') setPubsubDrawerOpen(true);
                if (id === 'sqladmin') setCloudSqlDrawerOpen(true);
                if (id === 'dataproc') setDataprocDrawerOpen(true);
                if (id === 'bigquery') setBigQueryDrawerOpen(true);
                if (id === 'bigtable') setBigtableDrawerOpen(true);
                if (id === 'gke') setGkeDrawerOpen(true);
                if (id === 'appengine') setAppEngineDrawerOpen(true);
                if (id === 'cloudfunctions' || id === 'serverless') setServerlessDrawerOpen(true);
              }}
              onInstallDependency={handleInstallDependency}
            />
          ))}
        </Box>
      )}

      <StorageManagerDrawer open={drawerOpen} onClose={() => setDrawerOpen(false)} />
      <IamManagerDrawer open={iamDrawerOpen} onClose={() => setIamDrawerOpen(false)} />
      <ComputeManagerDrawer open={computeDrawerOpen} onClose={() => setComputeDrawerOpen(false)} />
      <NetworkManagerDrawer open={networkDrawerOpen} onClose={() => setNetworkDrawerOpen(false)} />
      <FirestoreManagerDrawer open={firestoreDrawerOpen} onClose={() => setFirestoreDrawerOpen(false)} />
      <PubSubManagerDrawer open={pubsubDrawerOpen} onClose={() => setPubsubDrawerOpen(false)} />
      <CloudSqlManagerDrawer open={cloudSqlDrawerOpen} onClose={() => setCloudSqlDrawerOpen(false)} />
      <DataprocManagerDrawer 
        open={dataprocDrawerOpen} 
        onClose={() => setDataprocDrawerOpen(false)} 
        onOpenStorage={() => setDrawerOpen(true)}
        onOpenBigQuery={() => setBigQueryDrawerOpen(true)}
      />
      <ServerlessManagerDrawer 
        open={serverlessDrawerOpen} 
        onClose={() => setServerlessDrawerOpen(false)} 
        isBuildpacksEnabled={isBuildpacksEnabled}
        onEnableBuildpacks={async () => { if (missingPack) await handleInstallDependency('pack'); toggleSetting('MINISKY_SERVERLESS_BACKEND_ENABLED', isBuildpacksEnabled); }}
        missingPack={missingPack}
        onInstallPack={() => handleInstallDependency('pack')}
      />
      <BigQueryManagerDrawer open={bigQueryDrawerOpen} onClose={() => setBigQueryDrawerOpen(false)} />
      <BigtableManagerDrawer open={bigtableDrawerOpen} onClose={() => setBigtableDrawerOpen(false)} />
      <GKEManagerDrawer open={gkeDrawerOpen} onClose={() => setGkeDrawerOpen(false)} />
      <AppEngineManagerDrawer open={appEngineDrawerOpen} onClose={() => setAppEngineDrawerOpen(false)} />

      <style>{`
        @keyframes fadeIn {
          from { opacity: 0; transform: translateY(10px); }
          to { opacity: 1; transform: translateY(0); }
        }
      `}</style>
    </Box>
  );
}
