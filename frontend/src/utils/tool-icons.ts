/**
 * Tool Icons Utility
 * Maps tool names and match types to icons for better UI display
 */
import i18n from '@/i18n'

const t = (key: string) => i18n.global.t(key)

// Tool name to icon mapping
export const toolIcons: Record<string, string> = {
    multi_kb_search: 'search',
    knowledge_search: 'book',
    grep_chunks: 'search',
    get_chunk_detail: 'file',
    list_knowledge_bases: 'folder',
    list_knowledge_chunks: 'layers',
    get_document_info: 'info-circle',
    query_knowledge_graph: 'relation',
    think: 'ai-search',
    todo_write: 'task',
};

// Match type internal keys for icon mapping
const matchTypeIconKeys: Record<string, string> = {
    vector: 'focus',
    keyword: 'text',
    adjacent: 'pin',
    history: 'history',
    parent: 'arrow-up',
    relation: 'link',
    graph: 'relation',
};

// Match type to icon mapping (keys match backend API response)
export const matchTypeIcons: Record<string, string> = {
    'Vector Match': 'focus',
    'Keyword Match': 'text',
    'Adjacent Chunk Match': 'pin',
    'History Match': 'history',
    'Parent Chunk Match': 'arrow-up',
    'Relation Chunk Match': 'link',
    'Graph Match': 'relation',
};

// Get icon for a tool name
export function getToolIcon(toolName: string): string {
    return toolIcons[toolName] || 'tools';
}

// Get icon for a match type
export function getMatchTypeIcon(matchType: string): string {
    return matchTypeIcons[matchType] || matchTypeIconKeys[matchType] || 'pin';
}

// Tool name to i18n key mapping
const toolDisplayNameKeys: Record<string, string> = {
    multi_kb_search: 'tools.multiKbSearch',
    knowledge_search: 'tools.knowledgeSearch',
    grep_chunks: 'tools.grepChunks',
    get_chunk_detail: 'tools.getChunkDetail',
    list_knowledge_chunks: 'tools.listKnowledgeChunks',
    list_knowledge_bases: 'tools.listKnowledgeBases',
    get_document_info: 'tools.getDocumentInfo',
    query_knowledge_graph: 'tools.queryKnowledgeGraph',
    think: 'tools.think',
    todo_write: 'tools.todoWrite',
};

// Get tool display name (user-friendly, localized)
export function getToolDisplayName(toolName: string): string {
    const key = toolDisplayNameKeys[toolName];
    if (key) return t(key);

    // Format MCP tool names: "mcp_service_tool" → "Service Tool"
    if (toolName.startsWith('mcp_')) {
        const parts = toolName.slice(4).split('_');
        return parts.map(p => p.charAt(0).toUpperCase() + p.slice(1)).join(' ');
    }

    return toolName;
}

