import { NextResponse } from "next/server";
import OpenAI from "openai";

const model = process.env.DEMO_MODEL ?? "meta/llama-3.3-70b-instruct";
const ingestionUrl =
  process.env.TRACELIGHT_API_URL ?? "http://localhost:8080";

export async function POST() {
  const apiKey = process.env.NVIDIA_API_KEY;

  if (!apiKey) {
    return NextResponse.json(
      { error: "NVIDIA_API_KEY is not configured" },
      { status: 500 }
    );
  }

  const client = new OpenAI({
    apiKey,
    baseURL: "https://integrate.api.nvidia.com/v1",
  });

  const traceId = crypto.randomUUID();
  const startedAt = Date.now();

  try {
    const response = await client.chat.completions.create({
      model,
      messages: [
        {
          role: "user",
          content: "Explain what an event-driven architecture is in two sentences.",
        },
      ],
      temperature: 0.2,
      max_tokens: 150,
    });

    const durationMs = Date.now() - startedAt;
    const usage = response.usage;

    await sendTrace(ingestionUrl, {
      trace_id: traceId,
      provider_response_id: response.id ?? null,
      model: response.model ?? model,
      prompt_tokens: usage?.prompt_tokens ?? 0,
      completion_tokens: usage?.completion_tokens ?? 0,
      total_tokens: usage?.total_tokens ?? 0,
      duration_ms: durationMs,
      success: true,
      error: null,
      timestamp: new Date().toISOString(),
    });

    return NextResponse.json({
      trace_id: traceId,
      answer: response.choices[0]?.message?.content ?? "",
      model: response.model ?? model,
      duration_ms: durationMs,
    });
  } catch (error) {
    const durationMs = Date.now() - startedAt;

    await sendTrace(ingestionUrl, {
      trace_id: traceId,
      provider_response_id: null,
      model,
      prompt_tokens: 0,
      completion_tokens: 0,
      total_tokens: 0,
      duration_ms: durationMs,
      success: false,
      error: {
        name: error instanceof Error ? error.name : "UnknownError",
        message: error instanceof Error ? error.message : String(error),
      },
      timestamp: new Date().toISOString(),
    });

    return NextResponse.json(
      { error: "Model request failed", trace_id: traceId },
      { status: 502 }
    );
  }
}

async function sendTrace(url: string, payload: unknown) {
  const response = await fetch(`${url}/trace`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
    cache: "no-store",
  });

  if (!response.ok) {
    throw new Error(`Trace ingestion failed with status ${response.status}`);
  }
}