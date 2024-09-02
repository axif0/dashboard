import React, { useState, useEffect, useMemo } from 'react';
import Panel from '@/components/panel';
import { Menu, Button, Spin, Alert, Select } from 'antd';
import metricsData from './demomatrics.json';
import { getPods, PodsResponse, MetricsData, getMetrics } from '@/services/metrics';

const typedMetricsData: MetricsData = metricsData;

const Metrics: React.FC = () => {
  const [selectedApp, setSelectedApp] = useState<string | null>(null);
  const [selectedPod, setSelectedPod] = useState<string | null>(null);
  const [pods, setPods] = useState<PodsResponse | null>(null);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedMetric, setSelectedMetric] = useState<string | null>(null);
  const [metrics, setMetrics] = useState<string | null>(null);

  useEffect(() => {
    if (selectedApp) {
      console.log('Fetching pods for app:', selectedApp);
      fetchPods(selectedApp);
    }
  }, [selectedApp]);

  const fetchPods = async (appName: string) => {
    setLoading(true);
    setError(null);
    try {
      const response = await getPods(appName);
      console.log('Fetched pods:', response);
      setPods(response);
      setSelectedPod(null); // Reset the selected pod when fetching new pods
    } catch (error) {
      console.error('Error fetching pods:', error);
      setError('Failed to fetch pods. Please try again later.');
    } finally {
      setLoading(false);
    }
  };

  const handleAppClick = (key: string) => {
    console.log('App clicked:', key);
    setSelectedApp(key);
    setSelectedPod(null); // Reset the selected pod when selecting a new app
    setSelectedMetric(null); // Reset the selected metric when selecting a new app
    setMetrics(null); // Reset the metrics when selecting a new app
  };

  const handlePodSelect = (key: string) => {
    console.log('Pod selected:', key);
    setSelectedPod(key);
    setMetrics(null); // Reset the metrics when selecting a new pod
  };

  const handleMetricSelect = (key: string) => {
    console.log('Metric selected:', key);
    setSelectedMetric(key);
    setMetrics(null); // Reset the metrics when selecting a new metric
  };

  const fetchMetrics = async () => {
    if (selectedApp && selectedPod && selectedMetric) {
      try {
        const metricsData = await getMetrics(selectedApp, selectedPod, selectedMetric);
        setMetrics(metricsData);
        console.log(metricsData);
      } catch (error) {
        console.error('Failed to fetch metrics:', error);
      }
    }
  };

  useEffect(() => {
    fetchMetrics();
  }, [selectedApp, selectedPod, selectedMetric]);

  const menuItems = useMemo(() => {
    return Object.keys(typedMetricsData).map((key) => ({
      key,
      label: (
        <Button onClick={() => handleAppClick(key)} aria-haspopup="true" aria-expanded={selectedApp === key}>
          {key}
        </Button>
      ),
    }));
  }, [typedMetricsData, selectedApp]);

  const podOptions = useMemo(() => {
    if (pods && selectedApp) {
      console.log('Generating pod options for app:', selectedApp);
      const appPods = pods[selectedApp];
      if (appPods) {
        const nestedPods = Object.values(appPods).flat();
        if (Array.isArray(nestedPods)) {
          return nestedPods.map((pod) => ({
            value: pod.name,
            label: pod.name,
          }));
        } else {
          console.error('Expected nested pods to be an array, but got:', nestedPods);
          return [];
        }
      } else {
        console.error('No pods found for app:', selectedApp);
        return [];
      }
    }
    return [];
  }, [pods, selectedApp]);

  return (
    <Panel>
      <div style={{ display: 'flex' }}>
        <Menu
          mode="inline"
          style={{
            padding: '10px',
            width: '240px',
            height: '100%',
            maxHeight: '550px',
            overflowY: 'auto',
            borderRight: 0,
          }}
          items={menuItems}
        />
        <div style={{ marginLeft: '20px', flexGrow: 1 }}>
          {loading && <Spin size="large" />}
          {error && <Alert message={error} type="error" />}
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: '20px' }}>
            <h4>Metrics Information:</h4>
            <div style={{ display: 'flex', gap: '10px' }}>
              {selectedApp && (
                <Select
                  style={{ width: '200px' }}
                  placeholder="Select a pod"
                  value={selectedPod}
                  onChange={handlePodSelect}
                  options={podOptions}
                />
              )}
              {selectedApp && (
                <Select
                  style={{ width: '200px' }}
                  placeholder="Select a metric"
                  value={selectedMetric}
                  onChange={handleMetricSelect}
                  options={Object.entries(typedMetricsData[selectedApp]).map(([subKey, value]) => ({
                    value: subKey,
                    label: `${subKey}: ${value.Type}`,
                  }))}
                />
              )}
            </div>
          </div>
          <div style={{ marginTop: '20px', padding: '10px', border: '1px solid #ccc', borderRadius: '4px' }}>
            <pre>{metrics}</pre>
          </div>
        </div>
      </div>
    </Panel>
  );
};

export default Metrics;
