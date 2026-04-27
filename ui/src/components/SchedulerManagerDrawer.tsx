import { useState } from 'react';
import { 
  Drawer, Box, Typography, TextField, Button, MenuItem, 
  Stack, Divider, Alert, Tab, Tabs
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import { useProjectContext } from '../contexts/ProjectContext';

interface Props {
  open: boolean;
  onClose: () => void;
  onCreated: () => void;
}

export default function SchedulerManagerDrawer({ open, onClose, onCreated }: Props) {
  const { activeProject } = useProjectContext();
  const [activeTab, setActiveTab] = useState(0);
  const [form, setForm] = useState({
    id: '',
    description: '',
    schedule: '* * * * *',
    timeZone: 'UTC',
    // HTTP Target
    httpUri: `http://localhost:8080/v1/projects/${activeProject}/topics/test-topic:publish`,
    httpMethod: 'POST',
    httpBody: '{"messages":[{"data":"SGVsbG8gV29ybGQ="}]}',
    // PubSub Target
    pubsubTopic: `projects/${activeProject}/topics/test-topic`,
    pubsubData: 'SGVsbG8gU2NoZWR1bGVy',
  });
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const location = 'us-central1';

  const handleSubmit = async () => {
    setLoading(true);
    setError(null);

    const job: any = {
      name: form.id,
      description: form.description,
      schedule: form.schedule,
      timeZone: form.timeZone,
    };

    if (activeTab === 0) {
      job.httpTarget = {
        uri: form.httpUri,
        httpMethod: form.httpMethod,
        body: form.httpBody,
      };
    } else if (activeTab === 1) {
      job.pubsubTarget = {
        topicName: form.pubsubTopic,
        data: form.pubsubData,
      };
    }

    try {
      const res = await fetch(`/api/manage/scheduler/projects/${activeProject}/locations/${location}/jobs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(job),
      });

      if (!res.ok) throw new Error(await res.text());

      onCreated();
      onClose();
      setForm({ ...form, id: '', description: '' });
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ width: 450, p: 4 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
          <Typography variant="h5" sx={{ fontWeight: 500 }}>Create Scheduler Job</Typography>
          <Button onClick={onClose}><CloseIcon /></Button>
        </Box>

        <Divider sx={{ mb: 4 }} />

        {error && <Alert severity="error" sx={{ mb: 3 }}>{error}</Alert>}

        <Stack spacing={3}>
          <TextField 
            label="Job ID" 
            fullWidth 
            value={form.id}
            onChange={(e) => setForm({ ...form, id: e.target.value })}
            placeholder="e.g. my-daily-job"
          />

          <TextField 
            label="Description" 
            fullWidth 
            multiline 
            rows={2}
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
          />

          <Box sx={{ display: 'flex', gap: 2 }}>
            <TextField 
              label="Schedule (Cron)" 
              sx={{ flexGrow: 1 }}
              value={form.schedule}
              onChange={(e) => setForm({ ...form, schedule: e.target.value })}
              helperText="Standard unix-cron format"
            />
            <TextField 
              label="Time Zone" 
              select 
              sx={{ width: 120 }}
              value={form.timeZone}
              onChange={(e) => setForm({ ...form, timeZone: e.target.value })}
            >
              <MenuItem value="UTC">UTC</MenuItem>
              <MenuItem value="America/New_York">EST</MenuItem>
            </TextField>
          </Box>

          <Box>
            <Typography variant="subtitle2" sx={{ mb: 1, color: '#5f6368' }}>Target Type</Typography>
            <Tabs value={activeTab} onChange={(_, v) => setActiveTab(v)} sx={{ mb: 2 }}>
              <Tab label="HTTP" />
              <Tab label="Pub/Sub" />
            </Tabs>

            {activeTab === 0 && (
              <Stack spacing={2}>
                <TextField 
                  label="URL" 
                  fullWidth 
                  value={form.httpUri}
                  onChange={(e) => setForm({ ...form, httpUri: e.target.value })}
                />
                <TextField 
                  label="HTTP Method" 
                  select 
                  fullWidth
                  value={form.httpMethod}
                  onChange={(e) => setForm({ ...form, httpMethod: e.target.value })}
                >
                  <MenuItem value="GET">GET</MenuItem>
                  <MenuItem value="POST">POST</MenuItem>
                  <MenuItem value="PUT">PUT</MenuItem>
                </TextField>
                <TextField 
                  label="Body (JSON)" 
                  fullWidth 
                  multiline 
                  rows={4}
                  value={form.httpBody}
                  onChange={(e) => setForm({ ...form, httpBody: e.target.value })}
                  placeholder='{"key": "value"}'
                />
              </Stack>
            )}

            {activeTab === 1 && (
              <Stack spacing={2}>
                <TextField 
                  label="Topic Name" 
                  fullWidth 
                  value={form.pubsubTopic}
                  onChange={(e) => setForm({ ...form, pubsubTopic: e.target.value })}
                  placeholder="projects/{project}/topics/{topic}"
                />
                <TextField 
                  label="Data (Base64)" 
                  fullWidth 
                  multiline 
                  rows={4}
                  value={form.pubsubData}
                  onChange={(e) => setForm({ ...form, pubsubData: e.target.value })}
                />
              </Stack>
            )}
          </Box>
        </Stack>

        <Box sx={{ mt: 6, display: 'flex', gap: 2 }}>
          <Button variant="outlined" fullWidth onClick={onClose}>Cancel</Button>
          <Button 
            variant="contained" 
            fullWidth 
            onClick={handleSubmit}
            disabled={loading || !form.id || !form.schedule}
          >
            {loading ? 'Creating...' : 'Create Job'}
          </Button>
        </Box>
      </Box>
    </Drawer>
  );
}
