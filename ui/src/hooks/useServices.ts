import { useState, useEffect } from 'react';

export type Service = {
  id: string;
  name: string;
  label: string;
  status: string;
  port: number | null;
  description: string;
  missingDeps?: string[];
};

export function useServices() {
  const [services, setServices] = useState<Service[]>([]);
  const [settings, setSettings] = useState<any>({});

  const loadData = async () => {
    try {
      const res = await fetch('/api/services');
      if (res.ok) {
        setServices(await res.json());
      }
      const setRes = await fetch('/api/settings');
      if (setRes.ok) {
        setSettings(await setRes.json());
      }
    } catch (e) {
      console.error("error loading UI data", e);
    }
  };

  useEffect(() => {
    loadData();
    const interval = setInterval(loadData, 3000);
    return () => clearInterval(interval);
  }, []);

  const handleStartContainer = async (id: string, projectID?: string) => {
    let url = `/api/services/${id}/start`;
    if (projectID) {
      url += `?project=${encodeURIComponent(projectID)}`;
    }
    await fetch(url, { method: 'POST' });
    loadData();
  };

  const handleStopContainer = async (id: string) => {
    await fetch(`/api/services/${id}/stop`, { method: 'POST' });
    loadData();
  };

  const toggleSetting = async (key: string, currentVal: boolean) => {
    try {
      const res = await fetch('/api/settings', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ [key]: !currentVal })
      });
      if (!res.ok) {
        const errText = await res.text();
        alert(`Failed to update setting: ${errText}`);
      }
      loadData();
    } catch (e: any) {
      alert(`Error updating setting: ${e.message}`);
    }
  };

  const handleInstallDependency = async (id: string) => {
    try {
      const res = await fetch(`/api/manage/system/install-dependency/${id}`, { method: 'POST' });
      if (!res.ok) {
        const errText = await res.text();
        alert(`Installation failed: ${errText}`);
      } else {
        alert(`${id} installed successfully! You can now enable the service.`);
      }
      loadData();
    } catch (e: any) {
      alert(`Error installing dependency: ${e.message}`);
    }
  };

  return { services, settings, handleStartContainer, handleStopContainer, toggleSetting, handleInstallDependency };
}
