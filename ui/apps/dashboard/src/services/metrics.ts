import axios from 'axios';

export interface Pod {
  name: string;
  // Add other properties of a pod if necessary
}
export interface PodsResponse {
  [appName: string]: {
    [appName: string]: Pod[];
  };
}
interface MetricInfo {
    Name: string;
    Type: string;
  }
  
export interface MetricsCategory {
    [key: string]: MetricInfo;
  }
  
export interface MetricsData {
    [key: string]: MetricsCategory;
  }

export async function getPods(appName: string): Promise<PodsResponse> {
    try {
        const response = await axios.get(`/api/v1/pods/${appName}`);
        return response.data;
    } catch (error) {
        console.error('Failed to fetch pods:', error);
        throw error;
    }
}

export async function getMetrics(appName: string, podName: string, referenceName: string): Promise<string> {
    try {
        const response = await axios.get(`/api/v1/metrics/${appName}/${podName}/${referenceName}`);
        return response.data;
    } catch (error) {
        console.error('Failed to fetch metrics:', error);
        throw error;
    }
}