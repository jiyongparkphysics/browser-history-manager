import React, { useEffect, useRef, useState } from 'react'

export default function SearchBar({
  currentQuery = '',
  onSearch,
  disabled,
  placeholder = 'Search URLs and titles... (Enter to search)',
  ariaLabel = 'Search history',
}) {
  const [query, setQuery] = useState('')
  const composingRef = useRef(false)

  useEffect(() => {
    setQuery(currentQuery)
  }, [currentQuery])

  const handleSubmit = (e) => {
    e.preventDefault()
    onSearch(query)
  }

  const handleClear = () => {
    setQuery('')
    onSearch('')
  }

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && !composingRef.current) {
      e.preventDefault()
      onSearch(query)
    }
  }

  return (
    <form className="search-bar" onSubmit={handleSubmit}>
      <span className="material-symbols-outlined search-icon">search</span>
      <input
        type="text"
        placeholder={placeholder}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onKeyDown={handleKeyDown}
        onCompositionStart={() => { composingRef.current = true }}
        onCompositionEnd={() => { composingRef.current = false }}
        disabled={disabled}
        aria-label={ariaLabel}
      />
      {query && !disabled && (
        <button
          type="button"
          className="search-clear-btn"
          onClick={handleClear}
          aria-label="Clear search"
        >
          <span className="material-symbols-outlined">close</span>
        </button>
      )}
    </form>
  )
}
