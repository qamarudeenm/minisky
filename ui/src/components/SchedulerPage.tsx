import { useState, useEffect } from 'react';
import { 
  Box, Typography, Button, Paper, Table, TableBody, TableCell, 
  TableContainer, TableHead, TableRow, IconButton, Tooltip, 
  Chip, CircularProgress
} from '@mui/material';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import DeleteIcon from '@mui/icons-material/Delete';
import PauseIcon from '@mui/icons-material/Pause';
import RefreshIcon from '@mui/icons-material/Refresh';
import AddIcon from '@mui/icons-material/Add';
import SchedulerManagerDrawer from './SchedulerManagerDrawer';

interface Job {
  name: string;
  schedule: string;
  state: string;
  lastAttemptTime?: string;
  status?: {
    code: number;
    message: string;
  };
}

export default function SchedulerPage() {
  const [jobs, setJobs] = useState<Job[]>([]);
  const [loading, setLoading] = useState(true);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const project = 'local-dev-project';
  const location = 'us-central1';

  const fetchJobs = async () => {
    setLoading(true);
    try {
      const res = await fetch(`/api/manage/scheduler/projects/${project}/locations/${location}/jobs`);
      const data = await res.json();
      setJobs(data.jobs || []);
    } catch (err) {
      console.error('Failed to fetch jobs:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchJobs();
  }, []);

  const handleRunNow = async (jobName: string) => {
    try {
      await fetch(`/api/manage/scheduler/${jobName}:run`, { method: 'POST' });
      fetchJobs();
    } catch (err) {
      console.error('Failed to run job:', err);
    }
  };

  const handlePause = async (jobName: string) => {
    try {
      await fetch(`/api/manage/scheduler/${jobName}:pause`, { method: 'POST' });
      fetchJobs();
    } catch (err) {
      console.error('Failed to pause job:', err);
    }
  };

  const handleResume = async (jobName: string) => {
    try {
      await fetch(`/api/manage/scheduler/${jobName}:resume`, { method: 'POST' });
      fetchJobs();
    } catch (err) {
      console.error('Failed to resume job:', err);
    }
  };

  const handleDelete = async (jobName: string) => {
    if (!confirm('Are you sure you want to delete this job?')) return;
    try {
      await fetch(`/api/manage/scheduler/${jobName}`, { method: 'DELETE' });
      fetchJobs();
    } catch (err) {
      console.error('Failed to delete job:', err);
    }
  };

  return (
    <Box>
      <Box sx={{ mb: 4, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Box>
          <Typography variant="h4" sx={{ fontWeight: 500, mb: 1 }}>Cloud Scheduler</Typography>
          <Typography variant="body1" sx={{ color: '#5f6368' }}>
            Managed cron job service. Triggers HTTP, Pub/Sub, or App Engine targets on a schedule.
          </Typography>
        </Box>
        <Box sx={{ display: 'flex', gap: 2 }}>
          <Button startIcon={<RefreshIcon />} onClick={fetchJobs}>Refresh</Button>
          <Button 
            variant="contained" 
            startIcon={<AddIcon />} 
            onClick={() => setDrawerOpen(true)}
            sx={{ borderRadius: '20px', px: 3 }}
          >
            Create Job
          </Button>
        </Box>
      </Box>

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
          <CircularProgress />
        </Box>
      ) : (
        <TableContainer component={Paper} sx={{ borderRadius: '8px', border: '1px solid #dadce0', boxShadow: 'none' }}>
          <Table>
            <TableHead sx={{ backgroundColor: '#f8f9fa' }}>
              <TableRow>
                <TableCell sx={{ fontWeight: 600 }}>Job ID</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>Schedule</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>State</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>Last Attempt</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>Result</TableCell>
                <TableCell sx={{ fontWeight: 600 }} align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {jobs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} align="center" sx={{ py: 4, color: '#80868b' }}>
                    No jobs found. Create one to get started.
                  </TableCell>
                </TableRow>
              ) : (
                jobs.map((job) => {
                  const id = job.name.split('/').pop();
                  return (
                    <TableRow key={job.name} sx={{ '&:hover': { backgroundColor: '#fcfcfc' } }}>
                      <TableCell sx={{ fontWeight: 500 }}>{id}</TableCell>
                      <TableCell><code>{job.schedule}</code></TableCell>
                      <TableCell>
                        <Chip 
                          label={job.state} 
                          size="small" 
                          color={job.state === 'ENABLED' ? 'success' : 'default'}
                          sx={{ fontWeight: 500, fontSize: '0.75rem' }}
                        />
                      </TableCell>
                      <TableCell sx={{ fontSize: '0.85rem', color: '#5f6368' }}>
                        {job.lastAttemptTime ? new Date(job.lastAttemptTime).toLocaleString() : 'Never'}
                      </TableCell>
                      <TableCell>
                        {job.status ? (
                          <Tooltip title={job.status.message}>
                            <Chip 
                              label={job.status.code === 0 ? 'Success' : 'Error'} 
                              size="small" 
                              variant="outlined"
                              color={job.status.code === 0 ? 'success' : 'error'}
                            />
                          </Tooltip>
                        ) : '-'}
                      </TableCell>
                      <TableCell align="right">
                        <Tooltip title="Force run now">
                          <IconButton onClick={() => handleRunNow(job.name)} color="primary"><PlayArrowIcon /></IconButton>
                        </Tooltip>
                        {job.state === 'ENABLED' ? (
                          <Tooltip title="Pause">
                            <IconButton onClick={() => handlePause(job.name)}><PauseIcon /></IconButton>
                          </Tooltip>
                        ) : (
                          <Tooltip title="Resume">
                            <IconButton onClick={() => handleResume(job.name)} color="success"><PlayArrowIcon /></IconButton>
                          </Tooltip>
                        )}
                        <Tooltip title="Delete">
                          <IconButton onClick={() => handleDelete(job.name)} color="error"><DeleteIcon /></IconButton>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <SchedulerManagerDrawer 
        open={drawerOpen} 
        onClose={() => setDrawerOpen(false)} 
        onCreated={fetchJobs}
      />
    </Box>
  );
}
