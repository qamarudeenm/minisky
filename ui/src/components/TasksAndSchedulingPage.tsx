import { useState } from 'react';
import { Box, Typography, Tabs, Tab, Paper } from '@mui/material';
import SchedulerPage from './SchedulerPage';
import CloudTasksPageContent from './CloudTasksPageContent';
import ScheduleIcon from '@mui/icons-material/Schedule';
import RocketLaunchIcon from '@mui/icons-material/RocketLaunch';
import BuildIcon from '@mui/icons-material/Build';
import { useServices } from '../hooks/useServices';
import ServiceCard from './ServiceCard';
import CloudBuildDrawer from './CloudBuildDrawer';
import { useProjectContext } from '../contexts/ProjectContext';

export default function TasksAndSchedulingPage() {
  const [tab, setTab] = useState(0);
  const { activeProject } = useProjectContext();
  const { 
    services, settings, handleStartContainer, handleStopContainer, toggleSetting, handleInstallDependency 
  } = useServices();
  
  const [buildDrawerOpen, setBuildDrawerOpen] = useState(false);

  const buildService = services.find(s => s.id === 'cloudbuild');

  return (
    <Box>
      <Box sx={{ mb: 4 }}>
        <Typography variant="h4" sx={{ fontWeight: 500, mb: 1 }}>Tasks & Integration</Typography>
        <Typography variant="body1" sx={{ color: '#5f6368' }}>
          Manage asynchronous task execution, background workers, and scheduled jobs.
        </Typography>
      </Box>

      <Paper variant="outlined" sx={{ mb: 4, borderRadius: '12px', overflow: 'hidden', border: '1px solid #dadce0' }}>
        <Tabs 
          value={tab} 
          onChange={(_, v) => setTab(v)}
          sx={{ 
            px: 2, 
            pt: 1, 
            backgroundColor: '#f8f9fa',
            borderBottom: '1px solid #dadce0',
            '& .MuiTab-root': {
              textTransform: 'none',
              fontWeight: 500,
              fontSize: '0.95rem',
              minHeight: '48px'
            }
          }}
        >
          <Tab icon={<RocketLaunchIcon sx={{ fontSize: '1.2rem', mr: 1 }} />} iconPosition="start" label="Cloud Tasks" />
          <Tab icon={<ScheduleIcon sx={{ fontSize: '1.2rem', mr: 1 }} />} iconPosition="start" label="Cloud Scheduler" />
          <Tab icon={<BuildIcon sx={{ fontSize: '1.2rem', mr: 1 }} />} iconPosition="start" label="Cloud Build" />
        </Tabs>

        <Box sx={{ p: 4 }}>
          {tab === 0 && <CloudTasksPageContent />}
          {tab === 1 && <SchedulerPage />}
          {tab === 2 && buildService && (
            <Box sx={{ maxWidth: 400 }}>
              <ServiceCard 
                service={buildService} 
                idx={0} 
                settings={settings}
                onStartContainer={(id) => handleStartContainer(id, activeProject)}
                onStopContainer={handleStopContainer}
                onToggleSetting={toggleSetting}
                onManage={() => setBuildDrawerOpen(true)}
                onInstallDependency={handleInstallDependency}
              />
            </Box>
          )}
        </Box>
      </Paper>
      <CloudBuildDrawer open={buildDrawerOpen} onClose={() => setBuildDrawerOpen(false)} />
    </Box>
  );
}
