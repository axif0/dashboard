import { IResponse, karmadaClient } from './base';

export interface MetricInfo {
    help: string;
    type: 'COUNTER' | 'GAUGE' | 'SUMMARY' | 'HISTOGRAM';
}

export interface ClusterMetrics {
    [metricName: string]: MetricInfo;
}

export interface MetricsResponse {
    [clusterName: string]: ClusterMetrics;
}

 
export async function GetMetricsInfo(componentName: string, type: string): Promise<MetricsResponse> {
    // console.log("componentName", componentName, "type", type);
    const resp = await karmadaClient.get<IResponse<MetricsResponse>>(`/metrics/${componentName}?type=${type}`);
    // console.log("resp.data", resp.data);
    return resp.data as unknown as MetricsResponse;  // Convert to unknown first, then assert as MetricsResponse
}

export interface MetricValue {
    value: string;
    measure: string;
    labels: {
        [key: string]: string;
    };
}


export interface MetricDetails {
    name: string;
    values: MetricValue[];
}

export interface MetricDetailsResponse {
    details: {
        [timestamp: string]: MetricDetails;
    };
}


export async function GetMetricsDetails(componentName: string, podsName: string, metricName: string): Promise<MetricDetailsResponse> {
    console.log("componentName", componentName, "podsName", podsName, "metricName", metricName);
    const resp = await karmadaClient.get<MetricDetailsResponse>(`/metrics/${componentName}/${podsName}?type=details&mname=${metricName}`);
    console.log("resp.data GetMetricsDetails", resp.data);
    return resp.data;
}

export interface MetricDataResponse {
    [podName: string]: {
        currentTime: string;
        metrics: {
            [metricName: string]: {
                name: string;
                help: string;
                type: 'COUNTER' | 'GAUGE' | 'SUMMARY' | 'HISTOGRAM';
                values: MetricValue[];
            };
        };
    };
}

export async function GetMetricsData(componentName: string): Promise<{ status: number; data?: MetricDataResponse | any }> {
    console.log("componentName", componentName);
    const resp = await karmadaClient.get<MetricDataResponse>(`/metrics/${componentName}`);
    console.log("resp.data GetMetricsData", resp.data);

    let data = resp.data;

    if (typeof data === 'string') {
        try {
            data = JSON.parse(data);
        } catch (e) {
            console.log("string type error", e);
            return { status: 400, data: resp.data };
        }
    }

    if (data) {
        const isValidMetrics = Object.values(data).every(podMetrics =>
            podMetrics.currentTime &&
            podMetrics.metrics &&
            typeof podMetrics.metrics === 'object' &&
            Object.values(podMetrics.metrics).every(metric =>
                metric.name &&
                metric.help &&
                metric.type &&
                Array.isArray(metric.values)
            )
        );

        if (isValidMetrics) {
            return { status: 200, data };
        }
    }

    return { status: 400, data };
}
