import { useState } from 'react';
import { Box, Typography, Tabs, Tab, Paper } from '@mui/material';
import SchedulerPage from './SchedulerPage';
import CloudTasksPageContent from './CloudTasksPageContent';
import ScheduleIcon from '@mui/icons-material/Schedule';
import RocketLaunchIcon from '@mui/icons-material/RocketLaunch';

export default function TasksAndSchedulingPage() {
  const [tab, setTab] = useState(0);

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
        </Tabs>

        <Box sx={{ p: 4 }}>
          {tab === 0 && <CloudTasksPageContent />}
          {tab === 1 && <SchedulerPage />}
        </Box>
      </Paper>
    </Box>
  );
}
