import {
  Drawer, Box, Typography, IconButton, Paper, Alert
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import StorageIcon from '@mui/icons-material/Storage';
import { useProjectContext } from '../contexts/ProjectContext';

type Props = { open: boolean; onClose: () => void };

export default function DatastoreManagerDrawer({ open, onClose }: Props) {
  const { activeProject } = useProjectContext();

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: 500, bgcolor: '#f8f9fa' }}>
        <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 500, color: '#f9ab00' }}>Cloud Datastore</Typography>
            <Typography variant="caption" sx={{ color: '#5f6368' }}>Legacy NoSQL Console • {activeProject}</Typography>
          </Box>
          <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
        </Box>

        <Box sx={{ p: 3 }}>
          <Alert severity="info" sx={{ mb: 3 }}>
            Datastore is running in <strong>Firestore Datastore Mode</strong>. You can use the official gcloud CLI or Client SDKs to interact with this emulator.
          </Alert>

          <Paper elevation={0} sx={{ p: 4, textAlign: 'center', border: '1px dashed #dadce0', bgcolor: 'white' }}>
            <StorageIcon sx={{ fontSize: 48, color: '#dadce0', mb: 2 }} />
            <Typography variant="h6" sx={{ mb: 1 }}>No Entities to Display</Typography>
            <Typography variant="body2" sx={{ color: '#5f6368' }}>
              MiniSky's Datastore UI currently supports lifecycle management and proxy routing. 
              To manage entities, please use the <code>gcloud</code> emulator commands:
            </Typography>
            <Box sx={{ mt: 2, p: 2, bgcolor: '#f1f3f4', borderRadius: 1, fontFamily: 'monospace', fontSize: '0.8rem', textAlign: 'left' }}>
              export DATASTORE_EMULATOR_HOST=localhost:8081<br/>
              gcloud datastore indexes create ...
            </Box>
          </Paper>
        </Box>
      </Box>
    </Drawer>
  );
}
